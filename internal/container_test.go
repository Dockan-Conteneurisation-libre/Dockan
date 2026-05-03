package internal

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostSharedProxyPortsOnlyIncludesTranslatedPorts(t *testing.T) {
	got := hostSharedProxyPorts([]string{"8081:80", "9090:9090", "8443:443"})
	want := []string{"8081:80", "8443:443"}
	if len(got) != len(want) {
		t.Fatalf("hostSharedProxyPorts() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hostSharedProxyPorts() = %#v, want %#v", got, want)
		}
	}
}

func TestGateCommandUntilFilePreservesOriginalCommand(t *testing.T) {
	cmd := exec.Command("original", "arg one", "arg-two")
	cmd.Dir = "/tmp"

	gated := gateCommandUntilFile(cmd, "/tmp/dockan-ready")
	if gated.Args[0] != "sh" {
		t.Fatalf("Args[0] = %q, want sh", gated.Args[0])
	}
	if gated.Dir != cmd.Dir {
		t.Fatalf("Dir = %q, want %q", gated.Dir, cmd.Dir)
	}
	if len(gated.Args) < 7 {
		t.Fatalf("Args too short: %#v", gated.Args)
	}
	gotTail := gated.Args[len(gated.Args)-3:]
	wantTail := []string{"original", "arg one", "arg-two"}
	for i := range wantTail {
		if gotTail[i] != wantTail[i] {
			t.Fatalf("tail args = %#v, want %#v", gotTail, wantTail)
		}
	}
}

func TestLoadContainersFromRootReadsSelectedStore(t *testing.T) {
	root := t.TempDir()
	containerDir := filepath.Join(root, "containers", "prometheus-web")
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatal(err)
	}
	meta := "image=prometheus:local\nstatus=exited\npid=0\nports=9091:9090\n"
	if err := os.WriteFile(filepath.Join(containerDir, "meta.conf"), []byte(meta), 0644); err != nil {
		t.Fatal(err)
	}

	containers, err := LoadContainersFromRoot(root)
	if err != nil {
		t.Fatalf("LoadContainersFromRoot() error = %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("containers = %#v, want one", containers)
	}
	if containers[0].Name != "prometheus-web" || containers[0].Image != "prometheus:local" || containers[0].Ports != "9091:9090" {
		t.Fatalf("container = %#v", containers[0])
	}
}

func TestPrintContainersTableExpandsLongNames(t *testing.T) {
	var out bytes.Buffer
	printContainersTable(&out, true, []Container{{
		Name:   "nginx-proxy-manager-web",
		Status: "exited",
		PID:    168979,
		Image:  "nginx-proxy-manager:local",
		Ports:  "8084:80,8443:443,8181:81",
	}})

	text := out.String()
	if !strings.Contains(text, "nginx-proxy-manager-web  exited") {
		t.Fatalf("table did not keep a separator after long name:\n%s", text)
	}
}

func TestPrintScopedContainersTableExpandsLongNames(t *testing.T) {
	var out bytes.Buffer
	printScopedContainersTable(&out, []scopedContainerRow{{
		Store: "current",
		Container: Container{
			Name:   "nginx-proxy-manager-web",
			Status: "exited",
			PID:    168979,
			Image:  "nginx-proxy-manager:local",
			Ports:  "8084:80,8443:443,8181:81",
		},
	}})

	text := out.String()
	if !strings.Contains(text, "current    nginx-proxy-manager-web  exited") {
		t.Fatalf("scoped table did not keep separators after long fields:\n%s", text)
	}
}

func TestHealthcheckCommandParsesCommonForms(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "cmd shell form", raw: "CMD curl -f http://127.0.0.1:8000", want: "curl -f http://127.0.0.1:8000"},
		{name: "cmd shell", raw: "CMD-SHELL test -f /tmp/ready", want: "test -f /tmp/ready"},
		{name: "options before cmd", raw: "--interval=5s --timeout=2s CMD echo ok", want: "echo ok"},
		{name: "exec form", raw: `CMD ["curl", "-f", "http://127.0.0.1:8000"]`, want: `'curl' '-f' 'http://127.0.0.1:8000'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := healthcheckCommand(tt.raw)
			if err != nil {
				t.Fatalf("healthcheckCommand() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("healthcheckCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHealthcheckCommandHandlesNone(t *testing.T) {
	got, err := healthcheckCommand("NONE")
	if err != nil {
		t.Fatalf("healthcheckCommand() error = %v", err)
	}
	if got != "" {
		t.Fatalf("healthcheckCommand() = %q, want empty", got)
	}
}

func TestBridgeHealthcheckFallbackRewritesLoopback(t *testing.T) {
	meta := map[string]string{"networkIP": "10.87.13.197/24"}
	got := bridgeHealthcheckFallback("curl -f http://127.0.0.1:80/ || exit 1", meta)
	want := "curl -f http://10.87.13.197:80/ || exit 1"
	if got != want {
		t.Fatalf("bridgeHealthcheckFallback() = %q, want %q", got, want)
	}
}

func TestBridgeHealthcheckFallbackIgnoresNonLoopback(t *testing.T) {
	meta := map[string]string{"networkIP": "10.87.13.197/24"}
	if got := bridgeHealthcheckFallback("test -f /tmp/ready", meta); got != "" {
		t.Fatalf("bridgeHealthcheckFallback() = %q, want empty", got)
	}
}
