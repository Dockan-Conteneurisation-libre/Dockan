package internal

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type tarEntry struct {
	name     string
	typeflag byte
	linkname string
	body     string
}

func buildTar(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Typeflag: e.typeflag, Linkname: e.linkname, Mode: 0644}
		if e.typeflag == tar.TypeDir {
			hdr.Mode = 0755
		}
		if e.typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%s): %v", e.name, err)
		}
		if e.typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("Write(%s): %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	return buf.Bytes()
}

func assertNotExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Lstat(p); !os.IsNotExist(err) {
		t.Fatalf("%s must not exist (err=%v)", p, err)
	}
}

func assertFileContent(t *testing.T, p, want string) {
	t.Helper()
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", p, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", p, got, want)
	}
}

func TestExtractTarStreamConfinesAbsoluteSymlinkWrites(t *testing.T) {
	base := t.TempDir()
	dest := filepath.Join(base, "rootfs")
	outside := filepath.Join(base, "host")
	for _, dir := range []string{dest, outside} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	layer := buildTar(t, []tarEntry{
		{name: "etc", typeflag: tar.TypeSymlink, linkname: outside},
		{name: "etc/pwned", typeflag: tar.TypeReg, body: "owned"},
	})
	if err := extractTarStream(bytes.NewReader(layer), dest); err != nil {
		t.Fatalf("extractTarStream: %v", err)
	}
	assertNotExist(t, filepath.Join(outside, "pwned"))
	assertFileContent(t, filepath.Join(dest, outside, "pwned"), "owned")
}

func TestExtractTarStreamConfinesRelativeSymlinkWrites(t *testing.T) {
	base := t.TempDir()
	dest := filepath.Join(base, "a", "b", "rootfs")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}
	layer := buildTar(t, []tarEntry{
		{name: "esc", typeflag: tar.TypeSymlink, linkname: "../../../../.."},
		{name: "esc/pwned", typeflag: tar.TypeReg, body: "owned"},
	})
	if err := extractTarStream(bytes.NewReader(layer), dest); err != nil {
		t.Fatalf("extractTarStream: %v", err)
	}
	assertNotExist(t, filepath.Join(base, "pwned"))
	assertFileContent(t, filepath.Join(dest, "pwned"), "owned")
}

func TestExtractTarStreamDoesNotFollowSymlinkAtFileTarget(t *testing.T) {
	base := t.TempDir()
	dest := filepath.Join(base, "rootfs")
	secret := filepath.Join(base, "secret")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("host"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dest, "config")); err != nil {
		t.Fatal(err)
	}
	layer := buildTar(t, []tarEntry{{name: "config", typeflag: tar.TypeReg, body: "image"}})
	if err := extractTarStream(bytes.NewReader(layer), dest); err != nil {
		t.Fatalf("extractTarStream: %v", err)
	}
	assertFileContent(t, secret, "host")
	assertFileContent(t, filepath.Join(dest, "config"), "image")
}

func TestExtractTarStreamWhiteoutsStayInsideRootfs(t *testing.T) {
	base := t.TempDir()
	dest := filepath.Join(base, "rootfs")
	outside := filepath.Join(base, "host")
	for _, dir := range []string{dest, outside} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	victim := filepath.Join(outside, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	layer := buildTar(t, []tarEntry{
		{name: "lnk", typeflag: tar.TypeSymlink, linkname: outside},
		{name: "rel", typeflag: tar.TypeSymlink, linkname: "../host"},
		{name: "lnk/.wh.victim", typeflag: tar.TypeReg},
		{name: "rel/.wh.victim", typeflag: tar.TypeReg},
		{name: "lnk/.wh..wh..opq", typeflag: tar.TypeReg},
		{name: "rel/.wh..wh..opq", typeflag: tar.TypeReg},
	})
	if err := extractTarStream(bytes.NewReader(layer), dest); err != nil {
		t.Fatalf("extractTarStream: %v", err)
	}
	assertFileContent(t, victim, "keep")
}

func TestExtractTarStreamAppliesWhiteoutsInsideRootfs(t *testing.T) {
	dest := t.TempDir()
	lower := buildTar(t, []tarEntry{
		{name: "app", typeflag: tar.TypeDir},
		{name: "app/old", typeflag: tar.TypeReg, body: "x"},
		{name: "cache", typeflag: tar.TypeDir},
		{name: "cache/a", typeflag: tar.TypeReg, body: "a"},
	})
	upper := buildTar(t, []tarEntry{
		{name: "app/.wh.old", typeflag: tar.TypeReg},
		{name: "cache/.wh..wh..opq", typeflag: tar.TypeReg},
		{name: "cache/b", typeflag: tar.TypeReg, body: "b"},
	})
	for _, layer := range [][]byte{lower, upper} {
		if err := extractTarStream(bytes.NewReader(layer), dest); err != nil {
			t.Fatalf("extractTarStream: %v", err)
		}
	}
	assertNotExist(t, filepath.Join(dest, "app", "old"))
	assertNotExist(t, filepath.Join(dest, "cache", "a"))
	assertFileContent(t, filepath.Join(dest, "cache", "b"), "b")
}

func TestExtractTarStreamRejectsHardLinkOutsideLayer(t *testing.T) {
	dest := t.TempDir()
	layer := buildTar(t, []tarEntry{{name: "h", typeflag: tar.TypeLink, linkname: "../../etc/passwd"}})
	if err := extractTarStream(bytes.NewReader(layer), dest); err == nil {
		t.Fatal("hard link escaping the layer must be rejected")
	}
	assertNotExist(t, filepath.Join(dest, "h"))
}

func TestImportImageConfinesSymlinkWrites(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "host")
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatal(err)
	}
	payload := buildTar(t, []tarEntry{
		{name: "rootfs/lnk", typeflag: tar.TypeSymlink, linkname: outside},
		{name: "rootfs/lnk/pwned", typeflag: tar.TypeReg, body: "owned"},
	})
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := zw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(base, "image.tar.gz")
	if err := os.WriteFile(archive, gz.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(base, "imported")
	if err := ImportImage(archive, dest); err != nil {
		t.Fatalf("ImportImage: %v", err)
	}
	assertNotExist(t, filepath.Join(outside, "pwned"))
}

func TestSecureJoinStopsSymlinkLoops(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("b", filepath.Join(root, "a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a", filepath.Join(root, "b")); err != nil {
		t.Fatal(err)
	}
	if _, err := secureJoin(root, "a/x"); err == nil {
		t.Fatal("symlink loop must be reported")
	}
}

func sha256Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestSpoolVerifiedBlobAcceptsMatchingDigest(t *testing.T) {
	data := []byte("layer content")
	f, err := spoolVerifiedBlob(bytes.NewReader(data), sha256Digest(data), int64(len(data)), t.TempDir())
	if err != nil {
		t.Fatalf("spoolVerifiedBlob: %v", err)
	}
	defer discardTempFile(f)
	var got bytes.Buffer
	if _, err := got.ReadFrom(f); err != nil {
		t.Fatal(err)
	}
	if got.String() != string(data) {
		t.Fatalf("spooled content = %q", got.String())
	}
}

func TestSpoolVerifiedBlobRejectsTampering(t *testing.T) {
	data := []byte("layer content")
	cases := map[string]struct {
		body   []byte
		digest string
		size   int64
	}{
		"digest mismatch": {[]byte("tampered!!!!!"), sha256Digest(data), int64(len(data))},
		"size too large":  {append(data, 'x'), sha256Digest(data), int64(len(data))},
		"size too small":  {data[:4], sha256Digest(data), int64(len(data))},
		"bad digest":      {data, "sha256:../../evil", 0},
		"unknown algo":    {data, "md5:" + strings.Repeat("a", 32), 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			f, err := spoolVerifiedBlob(bytes.NewReader(tc.body), tc.digest, tc.size, dir)
			if err == nil {
				discardTempFile(f)
				t.Fatal("tampered blob must be rejected")
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("rejected blob must be removed, found %d entries", len(entries))
			}
		})
	}
}

func TestReadVerifiedOCIMetadata(t *testing.T) {
	doc := []byte(`{"config":{}}`)
	if _, err := readVerifiedOCIMetadata(bytes.NewReader(doc), sha256Digest(doc)); err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}
	if _, err := readVerifiedOCIMetadata(bytes.NewReader([]byte(`{"x":1}`)), sha256Digest(doc)); err == nil {
		t.Fatal("document with wrong digest must be rejected")
	}
}

func TestExtractLayerBlobHandlesGzipAndPlainTar(t *testing.T) {
	layer := buildTar(t, []tarEntry{{name: "hello", typeflag: tar.TypeReg, body: "hi"}})
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := zw.Write(layer); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	for name, blob := range map[string][]byte{"gzip": gz.Bytes(), "plain": layer} {
		t.Run(name, func(t *testing.T) {
			dest := t.TempDir()
			if err := extractLayerBlob(bytes.NewReader(blob), dest); err != nil {
				t.Fatalf("extractLayerBlob: %v", err)
			}
			assertFileContent(t, filepath.Join(dest, "hello"), "hi")
		})
	}
}
