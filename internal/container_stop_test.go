package internal

import (
	"os/exec"
	"testing"
	"time"
)

func TestParseProcStatGroup(t *testing.T) {
	state, group, ok := parseProcStatGroup("4242 (my (odd) cmd) S 1 4240 4240 0 -1 4194560")
	if !ok || state != "S" || group != 4240 {
		t.Fatalf("parseProcStatGroup = %q, %d, %v; want S, 4240, true", state, group, ok)
	}
	if _, _, ok := parseProcStatGroup("garbage"); ok {
		t.Fatal("malformed stat line must not parse")
	}
}

// Reproduces a sandbox whose launcher exits on SIGTERM while the workload
// ignores it (PID 1 of its namespace): stop must not leave it running.
func TestTerminateContainerGroupKillsWorkloadIgnoringSIGTERM(t *testing.T) {
	cmd := exec.Command("sh", "-c", `sh -c "trap '' TERM; sleep 60" & wait`)
	cmd.SysProcAttr = detachedSysProcAttr(false)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid
	waited := make(chan struct{})
	go func() {
		_ = cmd.Wait() // reap the leader; its exit status is irrelevant here
		close(waited)
	}()
	for deadline := time.Now().Add(3 * time.Second); !processGroupAlive(pid); {
		if time.Now().After(deadline) {
			t.Fatal("container group must be alive before stop")
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond) // let the inner shell install its trap
	if err := terminateContainerGroup(pid); err != nil {
		t.Fatalf("terminateContainerGroup: %v", err)
	}
	if containerRunning(pid) {
		t.Fatal("a process of the container group survived stop")
	}
	select {
	case <-waited:
	case <-time.After(3 * time.Second):
		t.Fatal("container leader was not reaped")
	}
}
