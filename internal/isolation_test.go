package internal

import (
	"path/filepath"
	"testing"
)

func TestOCIBubblewrapKeepsRootfsTmp(t *testing.T) {
	base := t.TempDir()
	cmd, err := ociRootfsCommand(IsolationBubblewrap, filepath.Join(base, "image"), filepath.Join(base, "rootfs"), "/", nil)
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
	cmd, err := ociRootfsCommand(IsolationBubblewrap, filepath.Join(base, "image"), filepath.Join(base, "rootfs"), "/", nil)
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

func TestOCIBubblewrapRunsCommandAsPID1(t *testing.T) {
	base := t.TempDir()
	cmd, err := ociRootfsCommand(IsolationBubblewrap, filepath.Join(base, "image"), filepath.Join(base, "rootfs"), "/", nil)
	if err != nil {
		t.Fatalf("ociRootfsCommand() error = %v", err)
	}
	for _, arg := range cmd.Args {
		if arg == "--as-pid-1" {
			return
		}
	}
	t.Fatalf("OCI bubblewrap command must run s6-style init as PID 1, got args %#v", cmd.Args)
}

func TestOCIBubblewrapAddsPrivateVolumeBinds(t *testing.T) {
	base := t.TempDir()
	cmd, err := ociRootfsCommand(IsolationBubblewrap, filepath.Join(base, "image"), filepath.Join(base, "rootfs"), "/", []VolumeBind{
		{Source: filepath.Join(base, "data"), Target: "/var/lib/postgresql"},
		{Source: filepath.Join(base, "config"), Target: "/config", ReadOnly: true},
	})
	if err != nil {
		t.Fatalf("ociRootfsCommand() error = %v", err)
	}
	args := cmd.Args
	assertArgSequence(t, args, "--bind", filepath.Join(base, "data"), "/var/lib/postgresql")
	assertArgSequence(t, args, "--ro-bind", filepath.Join(base, "config"), "/config")
}

func assertArgSequence(t *testing.T, args []string, want ...string) {
	t.Helper()
	for i := 0; i <= len(args)-len(want); i++ {
		ok := true
		for j := range want {
			if args[i+j] != want[j] {
				ok = false
				break
			}
		}
		if ok {
			return
		}
	}
	t.Fatalf("args %#v missing sequence %#v", args, want)
}
