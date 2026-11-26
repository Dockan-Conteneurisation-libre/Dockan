package internal

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net"
	"os/exec"
	"strings"
	"testing"
)

func TestRegistryIsLocalIgnoresDNS(t *testing.T) {
	t.Setenv(privateRegistriesEnv, "")
	for host, want := range map[string]bool{
		"localhost:5000":       true,
		"10.0.0.5:5000":        true,
		"[::1]:5000":           true,
		"registry.lan":         false,
		"registry-1.docker.io": false,
		"1.1.1.1":              false,
	} {
		if got := registryIsLocal(host); got != want {
			t.Errorf("registryIsLocal(%q) = %v, want %v", host, got, want)
		}
	}
	t.Setenv(privateRegistriesEnv, "registry.lan, nas.home:5000")
	if !registryIsLocal("registry.lan:443") || !registryIsLocal("nas.home:5000") {
		t.Fatal("registries listed by the user must be trusted with private addresses")
	}
	if registryIsLocal("other.lan") {
		t.Fatal("unlisted names must stay public")
	}
}

func TestIsPublicIPSpecialRanges(t *testing.T) {
	for _, s := range []string{"198.18.0.1", "192.0.2.1", "192.0.0.8", "203.0.113.9", "240.0.0.1",
		"255.255.255.255", "64:ff9b::a00:1", "2002:a00:1::1", "fec0::1", "::a00:1", "2001:db8::1"} {
		if isPublicIP(net.ParseIP(s)) {
			t.Errorf("%s must not be public", s)
		}
	}
	for _, s := range []string{"64:ff9b::101:101", "2002:101:101::1"} {
		if !isPublicIP(net.ParseIP(s)) {
			t.Errorf("%s embeds a public IPv4 address and must be public", s)
		}
	}
}

func TestAllProxyDoesNotDisableDialCheck(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("http_proxy", "")
	t.Setenv("ALL_PROXY", "socks5://10.0.0.9:1080")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	dial := policyDialContext(&net.Dialer{})
	conn, err := dial(withNetworkPolicy(context.Background(), false), "tcp", ln.Addr().String())
	if err == nil {
		conn.Close()
		t.Fatal("ALL_PROXY (ignored by Go's transport) must not disable the check")
	}
	t.Setenv("HTTPS_PROXY", "http://"+ln.Addr().String())
	conn, err = dial(withNetworkPolicy(context.Background(), false), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("the configured proxy itself must stay reachable: %v", err)
	}
	conn.Close()
}

func TestExtractLayerBlobCapsDecodedSize(t *testing.T) {
	previous := maxDecodedLayerBytes
	maxDecodedLayerBytes = 4096
	defer func() { maxDecodedLayerBytes = previous }()
	layer := buildTar(t, []tarEntry{{name: "big", typeflag: tar.TypeReg, body: strings.Repeat("0", 64*1024)}})
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := zw.Write(layer); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	err := extractLayerBlob(bytes.NewReader(gz.Bytes()), t.TempDir())
	if !errors.Is(err, errLayerTooLarge) {
		t.Fatalf("oversized layer: got %v, want errLayerTooLarge", err)
	}
}

func TestZstdLayerRejectsTrailingData(t *testing.T) {
	layer := buildTar(t, []tarEntry{{name: "a", typeflag: tar.TypeReg, body: "a"}})
	payload := append(layer, bytes.Repeat([]byte{1}, 2<<20)...)
	if _, err := exec.LookPath("zstd"); err != nil {
		t.Skip("zstd tool not installed")
	}
	blob := zstdCompress(t, payload)
	if err := extractLayerBlob(bytes.NewReader(blob), t.TempDir()); err == nil {
		t.Fatal("data hidden after the end of the archive must be rejected")
	}
}

func TestPinnedImageHasItsOwnStorePath(t *testing.T) {
	t.Setenv("DOCKAN_HOME", t.TempDir())
	digest := "sha256:" + strings.Repeat("c", 64)
	pinned := StoreImagePath("alpine@" + digest)
	tagged := StoreImagePath("alpine:sha256_" + strings.Repeat("c", 64))
	if pinned == tagged {
		t.Fatalf("pinned and tagged images share %s", pinned)
	}
}

func TestConfirmScopeReportsDeadContainer(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if err := confirmScope(cmd.Process.Pid, "dockan-never-started", RunOptions{}); err == nil {
		t.Fatal("a container that already exited must be reported")
	}
}
