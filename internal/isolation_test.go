package internal

import (
	"path/filepath"
	"testing"
)

func TestOCIBubblewrapKeepsRootfsTmp(t *testing.T) {
	base := t.TempDir()
	cmd, err := ociRootfsCommand(IsolationBubblewrap, filepath.Join(base, "image"), filepath.Join(base, "rootfs"), "/")
	if err != nil {
		t.Fatalf("ociRootfsCommand() error = %v", err)
	}
	for i := 0; i < len(cmd.Args)-1; i++ {
		if cmd.Args[i] == "--tmpfs" && cmd.Args[i+1] == "/tmp" {
			t.Fatalf("OCI bubblewrap command must keep rootfs /tmp, got args %#v", cmd.Args)
		}
	}
}

func TestOCIBubblewrapUnsharesPIDNamespace(t *testing.T) {
	base := t.TempDir()
	cmd, err := ociRootfsCommand(IsolationBubblewrap, filepath.Join(base, "image"), filepath.Join(base, "rootfs"), "/")
	if err != nil {
		t.Fatalf("ociRootfsCommand() error = %v", err)
	}
	for _, arg := range cmd.Args {
		if arg == "--unshare-pid" {
			return
		}
	}
	t.Fatalf("OCI bubblewrap command must create a PID namespace, got args %#v", cmd.Args)
}
