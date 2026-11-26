package internal

import (
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestResolveIsolationAutoRefusesSilentNone(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	method, err := ResolveIsolation(IsolationAuto)
	if !errors.Is(err, errNoIsolationAvailable) {
		t.Fatalf("ResolveIsolation(auto) = %q, %v; want errNoIsolationAvailable", method, err)
	}
	method, err = ResolveIsolation(IsolationNone)
	if err != nil || method != IsolationNone {
		t.Fatalf("explicit none must stay allowed, got %q, %v", method, err)
	}
}

func assertHardenedBubblewrap(t *testing.T, args []string) {
	t.Helper()
	for _, want := range append([]string{"--unshare-pid"}, hardenedBubblewrapArgs...) {
		found := false
		for _, arg := range args {
			if arg == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("bubblewrap args miss %s: %#v", want, args)
		}
	}
}

func TestOCIBubblewrapUnsharesIPCAndUTS(t *testing.T) {
	base := t.TempDir()
	cmd, err := ociRootfsCommand(IsolationBubblewrap, filepath.Join(base, "image"), filepath.Join(base, "rootfs"), "/", nil)
	if err != nil {
		t.Fatalf("ociRootfsCommand() error = %v", err)
	}
	assertHardenedBubblewrap(t, cmd.Args)
}

func TestLegacyBubblewrapIsHardened(t *testing.T) {
	base := t.TempDir()
	img := &Image{
		Path:        filepath.Join(base, "image"),
		RootfsDir:   filepath.Join(base, "image", "rootfs"),
		StartScript: filepath.Join(base, "image", "start.sh"),
		Meta:        map[string]string{},
	}
	cmd, err := isolationCommand(IsolationBubblewrap, img, nil)
	if err != nil {
		t.Fatalf("isolationCommand() error = %v", err)
	}
	assertHardenedBubblewrap(t, cmd.Args)
}

func TestDetachedSysProcAttrStartsNewSession(t *testing.T) {
	attr := detachedSysProcAttr(false)
	if !attr.Setsid || attr.Cloneflags != 0 {
		t.Fatalf("host network attr = %+v, want Setsid only", attr)
	}
	attr = detachedSysProcAttr(true)
	if !attr.Setsid || attr.Cloneflags != syscall.CLONE_NEWNET {
		t.Fatalf("bridge attr = %+v, want Setsid and CLONE_NEWNET", attr)
	}
}

// A background container must stay in the process group that "dockan stop"
// signals: nothing may survive a group kill.
func TestDetachedSessionIsKilledAsAGroup(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 60 & wait")
	cmd.SysProcAttr = detachedSysProcAttr(false)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid
	time.Sleep(200 * time.Millisecond)
	if err := signalContainerProcess(pid, syscall.SIGTERM); err != nil {
		t.Fatalf("signalContainerProcess: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("container shell must be terminated by the group signal")
	}
	// Orphaned members are reaped asynchronously by init: poll briefly.
	for deadline := time.Now().Add(3 * time.Second); syscall.Kill(-pid, 0) == nil; {
		if time.Now().After(deadline) {
			t.Fatal("a process of the container group survived the stop signal")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
