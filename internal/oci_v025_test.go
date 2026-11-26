package internal

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseOCIRefPinnedDigest(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	cases := []struct {
		raw, registry, repo string
	}{
		{"alpine@" + digest, "registry-1.docker.io", "library/alpine"},
		{"alpine:3.20@" + digest, "registry-1.docker.io", "library/alpine"},
		{"ghcr.io/org/app:1.0@" + digest, "ghcr.io", "org/app"},
		{"localhost:5000/app@" + digest, "localhost:5000", "app"},
	}
	for _, tc := range cases {
		ref := parseOCIRef(tc.raw)
		if ref.Registry != tc.registry || ref.Repository != tc.repo || ref.Digest != digest || ref.Tag != digest {
			t.Errorf("parseOCIRef(%q) = %+v", tc.raw, ref)
		}
	}
	if ref := parseOCIRef("alpine:3.20"); ref.Digest != "" || ref.Tag != "3.20" {
		t.Errorf("unpinned reference changed: %+v", ref)
	}
}

func TestFetchOCIManifestEnforcesPinnedDigest(t *testing.T) {
	manifest := []byte(`{"config":{"digest":"sha256:` + strings.Repeat("b", 64) + `"},"layers":[]}`)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	good := sha256Digest(manifest)
	ref := OCIRef{Registry: u.Host, Repository: "app", Tag: good, Digest: good}
	if _, _, err := fetchOCIManifest(ref); err != nil {
		t.Fatalf("pinned manifest rejected: %v", err)
	}
	bad := sha256Digest([]byte("other"))
	ref = OCIRef{Registry: u.Host, Repository: "app", Tag: bad, Digest: bad}
	if _, _, err := fetchOCIManifest(ref); err == nil {
		t.Fatal("manifest not matching the pinned digest must be rejected")
	}
	ref = OCIRef{Registry: u.Host, Repository: "app", Tag: "x", Digest: "sha256:../../evil"}
	if _, _, err := fetchOCIManifest(ref); err == nil {
		t.Fatal("malformed pinned digest must be rejected before any request")
	}
}

func TestIsPublicIP(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.0.252", "169.254.169.254",
		"100.64.0.1", "0.0.0.0", "::1", "fe80::1", "fd00::1", "::ffff:127.0.0.1", "224.0.0.1"} {
		if isPublicIP(net.ParseIP(s)) {
			t.Errorf("%s must not be public", s)
		}
	}
	for _, s := range []string{"1.1.1.1", "137.74.114.199", "2606:4700:4700::1111"} {
		if !isPublicIP(net.ParseIP(s)) {
			t.Errorf("%s must be public", s)
		}
	}
}

func TestCheckOutboundURLPolicy(t *testing.T) {
	public := withNetworkPolicy(context.Background(), false)
	for _, raw := range []string{"http://auth.example.org/token", "https://127.0.0.1/token",
		"https://10.0.0.5/token", "https://[::1]/token", "https://localhost/token", "https://169.254.169.254/latest"} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := checkOutboundURL(public, u); err == nil {
			t.Errorf("%s must be refused for a public registry", raw)
		}
	}
	u, err := url.Parse("https://1.1.1.1/token")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkOutboundURL(public, u); err != nil {
		t.Errorf("public HTTPS endpoint refused: %v", err)
	}
	local := withNetworkPolicy(context.Background(), true)
	u, err = url.Parse("http://192.168.0.10:5000/token")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkOutboundURL(local, u); err != nil {
		t.Errorf("a local registry may use a local token server: %v", err)
	}
}

func TestPolicyDialRefusesInternalAddresses(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("all_proxy", "")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	dial := policyDialContext(&net.Dialer{})
	public := withNetworkPolicy(context.Background(), false)
	if conn, err := dial(public, "tcp", ln.Addr().String()); err == nil {
		conn.Close()
		t.Fatal("dial to loopback must be refused for a public registry")
	} else if !errors.Is(err, errInternalAddress) {
		t.Fatalf("unexpected error: %v", err)
	}
	local := withNetworkPolicy(context.Background(), true)
	conn, err := dial(local, "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial for a local registry refused: %v", err)
	}
	conn.Close()
}

func TestFetchRegistryTokenRefusesInternalRealm(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if _, err := w.Write([]byte(`{"token":"x"}`)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()
	public := withNetworkPolicy(context.Background(), false)
	if _, err := fetchRegistryToken(public, srv.URL+"/token", "svc", "repo:x:pull"); err == nil {
		t.Fatal("an internal plain-HTTP realm must be refused for a public registry")
	}
	if hits.Load() != 0 {
		t.Fatal("the internal realm must not be contacted at all")
	}
	local := withNetworkPolicy(context.Background(), true)
	token, err := fetchRegistryToken(local, srv.URL+"/token", "svc", "repo:x:pull")
	if err != nil || token != "x" {
		t.Fatalf("local registry token = %q, %v", token, err)
	}
}

func zstdCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	path, err := exec.LookPath("zstd")
	if err != nil {
		t.Skip("zstd tool not installed")
	}
	cmd := exec.Command(path, "-q", "-c")
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("zstd: %v", err)
	}
	return out
}

func TestExtractLayerBlobZstd(t *testing.T) {
	layer := buildTar(t, []tarEntry{{name: "etc/os", typeflag: tar.TypeReg, body: "zstd ok"}})
	blob := zstdCompress(t, layer)
	dest := t.TempDir()
	if err := extractLayerBlob(bytes.NewReader(blob), dest); err != nil {
		t.Fatalf("extractLayerBlob: %v", err)
	}
	assertFileContent(t, filepath.Join(dest, "etc", "os"), "zstd ok")

	corrupt := append([]byte{}, blob[:len(blob)/2]...)
	if err := extractLayerBlob(bytes.NewReader(corrupt), t.TempDir()); err == nil {
		t.Fatal("a truncated zstd layer must be rejected")
	}
}

func TestZstdLayerWithoutToolExplainsWhatToInstall(t *testing.T) {
	previous := zstdCommand
	zstdCommand = "dockan-zstd-missing-for-test"
	defer func() { zstdCommand = previous }()
	blob := append([]byte{}, zstdMagic...)
	err := extractLayerBlob(bytes.NewReader(append(blob, 0, 0, 0)), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "zstd") {
		t.Fatalf("missing zstd must give an actionable error, got %v", err)
	}
}
