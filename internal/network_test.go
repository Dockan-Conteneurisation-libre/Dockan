package internal

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCreateBridgeNetworkStoresMetadata(t *testing.T) {
	t.Setenv("DOCKAN_HOME", filepath.Join(t.TempDir(), "store"))

	err := CreateNetworkWithOptions(NetworkOptions{
		Name:    "appnet",
		Driver:  "bridge",
		Subnet:  "10.90.0.0/24",
		Gateway: "10.90.0.1/24",
		Bridge:  "dockan-app",
	})
	if err != nil {
		t.Fatalf("CreateNetworkWithOptions() error = %v", err)
	}

	meta, err := loadNetwork("appnet")
	if err != nil {
		t.Fatalf("loadNetwork() error = %v", err)
	}
	if meta["driver"] != "bridge" || meta["subnet"] != "10.90.0.0/24" || meta["gateway"] != "10.90.0.1/24" || meta["bridge"] != "dockan-app" {
		t.Fatalf("network meta = %#v", meta)
	}
}

func TestDefaultComposeNetworkOptionsUsesBridge(t *testing.T) {
	opts := defaultComposeNetworkOptions("nextcloud-net")
	if opts.Driver != "bridge" {
		t.Fatalf("driver = %q, want bridge", opts.Driver)
	}
	if !strings.HasPrefix(opts.Subnet, "10.") || !strings.HasSuffix(opts.Subnet, ".0/24") {
		t.Fatalf("subnet = %q", opts.Subnet)
	}
	if !strings.HasSuffix(opts.Gateway, ".1/24") {
		t.Fatalf("gateway = %q", opts.Gateway)
	}
	if opts.Bridge == "" || len(opts.Bridge) > 15 {
		t.Fatalf("bridge = %q", opts.Bridge)
	}
}

func TestCreateBridgeNetworkRejectsBadCIDR(t *testing.T) {
	t.Setenv("DOCKAN_HOME", filepath.Join(t.TempDir(), "store"))

	err := CreateNetworkWithOptions(NetworkOptions{Name: "badnet", Driver: "bridge", Subnet: "bad", Gateway: "10.90.0.1/24"})
	if err == nil {
		t.Fatal("CreateNetworkWithOptions() expected error")
	}
}

func TestAllocateContainerAddress(t *testing.T) {
	t.Setenv("DOCKAN_HOME", filepath.Join(t.TempDir(), "store"))

	ip, gateway, err := AllocateContainerAddress("appnet", "web", "10.90.0.0/24", "10.90.0.1/24")
	if err != nil {
		t.Fatalf("AllocateContainerAddress() error = %v", err)
	}
	if ip == "" || ip == "10.90.0.1/24" {
		t.Fatalf("ip = %q", ip)
	}
	if gateway != "10.90.0.1" {
		t.Fatalf("gateway = %q", gateway)
	}
}

func TestVethNameFitsLinuxLimit(t *testing.T) {
	name := vethName("dh", "this-container-name-is-way-too-long")
	if len(name) > 15 {
		t.Fatalf("veth name too long: %q", name)
	}
}

func TestWriteNetworkHosts(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	imagePath := filepath.Join(base, "web.dockan")
	if err := InitImage(imagePath); err != nil {
		t.Fatal(err)
	}
	containerDir := filepath.Join(ContainersDir(), "demo-web")
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatal(err)
	}
	meta := map[string]string{
		"name":      "demo-web",
		"image":     "web:latest",
		"imagePath": imagePath,
		"pid":       strconv.Itoa(os.Getpid()),
		"status":    "running",
		"network":   "appnet",
		"networkIP": "10.89.0.2/24",
		"aliases":   "api,backend",
	}
	if err := WriteMeta(filepath.Join(containerDir, "meta.conf"), meta); err != nil {
		t.Fatal(err)
	}

	if err := WriteNetworkHosts("appnet"); err != nil {
		t.Fatalf("WriteNetworkHosts() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(imagePath, "rootfs", "etc", "hosts"))
	if err != nil {
		t.Fatal(err)
	}
	hosts := string(data)
	if !strings.Contains(hosts, "10.89.0.2 demo-web") || !strings.Contains(hosts, "10.89.0.2 web") || !strings.Contains(hosts, "10.89.0.2 api") || !strings.Contains(hosts, "10.89.0.2 backend") {
		t.Fatalf("hosts = %q", hosts)
	}
}
