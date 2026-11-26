package internal

import (
	"archive/tar"
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTarStreamRejectsDegenerateWhiteouts(t *testing.T) {
	for _, name := range []string{".wh.", "app/.wh.", "app/.wh.."} {
		t.Run(name, func(t *testing.T) {
			dest := t.TempDir()
			keep := filepath.Join(dest, "app", "keep")
			if err := os.MkdirAll(filepath.Dir(keep), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(keep, []byte("k"), 0644); err != nil {
				t.Fatal(err)
			}
			layer := buildTar(t, []tarEntry{{name: name, typeflag: tar.TypeReg}})
			if err := extractTarStream(bytes.NewReader(layer), dest); err == nil {
				t.Fatalf("whiteout %q must be rejected", name)
			}
			assertFileContent(t, keep, "k")
		})
	}
}

func TestExtractTarStreamReplacesPathsAcrossLayers(t *testing.T) {
	dest := t.TempDir()
	lower := buildTar(t, []tarEntry{
		{name: "data", typeflag: tar.TypeDir},
		{name: "data/old", typeflag: tar.TypeReg, body: "x"},
		{name: "conf", typeflag: tar.TypeReg, body: "file"},
	})
	upper := buildTar(t, []tarEntry{
		{name: "data", typeflag: tar.TypeSymlink, linkname: "/srv"},
		{name: "conf", typeflag: tar.TypeDir},
		{name: "conf/app.ini", typeflag: tar.TypeReg, body: "ini"},
	})
	for _, layer := range [][]byte{lower, upper} {
		if err := extractTarStream(bytes.NewReader(layer), dest); err != nil {
			t.Fatalf("extractTarStream: %v", err)
		}
	}
	if link, err := os.Readlink(filepath.Join(dest, "data")); err != nil || link != "/srv" {
		t.Fatalf("data = %q, %v; want symlink to /srv", link, err)
	}
	assertFileContent(t, filepath.Join(dest, "conf", "app.ini"), "ini")
}

func TestExtractTarStreamReRootsAbsoluteNames(t *testing.T) {
	dest := t.TempDir()
	layer := buildTar(t, []tarEntry{
		{name: "/etc/motd", typeflag: tar.TypeReg, body: "hi"},
		{name: "../../escape", typeflag: tar.TypeReg, body: "in"},
		{name: "..hidden", typeflag: tar.TypeReg, body: "dots"},
	})
	if err := extractTarStream(bytes.NewReader(layer), dest); err != nil {
		t.Fatalf("extractTarStream: %v", err)
	}
	assertFileContent(t, filepath.Join(dest, "etc", "motd"), "hi")
	assertFileContent(t, filepath.Join(dest, "escape"), "in")
	assertFileContent(t, filepath.Join(dest, "..hidden"), "dots")
}

func TestValidateOCILayerSize(t *testing.T) {
	for _, size := range []int64{0, -1, maxOCILayerBytes + 1} {
		if err := validateOCILayerSize(size); err == nil {
			t.Fatalf("size %d must be rejected", size)
		}
	}
	if err := validateOCILayerSize(1024); err != nil {
		t.Fatalf("valid size rejected: %v", err)
	}
}

func TestFetchOCIManifestChecksAnnouncedDigest(t *testing.T) {
	manifest := []byte(`{"config":{"digest":"sha256:` + string(bytes.Repeat([]byte("a"), 64)) + `"},"layers":[]}`)
	announced := sha256Digest(manifest)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Docker-Content-Digest", announced)
		if _, err := w.Write(manifest); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()
	previous := ociHTTPClient
	ociHTTPClient = srv.Client()
	defer func() { ociHTTPClient = previous }()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ref := OCIRef{Registry: u.Host, Repository: "library/test", Tag: "latest"}

	if _, _, err := fetchOCIManifest(ref); err != nil {
		t.Fatalf("manifest matching its announced digest rejected: %v", err)
	}
	announced = sha256Digest([]byte("something else"))
	if _, _, err := fetchOCIManifest(ref); err == nil {
		t.Fatal("manifest not matching its announced digest must be rejected")
	}
}
