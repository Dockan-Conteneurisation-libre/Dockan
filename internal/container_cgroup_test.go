package internal

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestScopeUnitName(t *testing.T) {
	got := scopeUnitName("web/app 1", "0123456789abcdef0123")
	if got != "dockan-web-app-1-0123456789ab" {
		t.Fatalf("scopeUnitName = %q", got)
	}
	long := scopeUnitName(strings.Repeat("x", 100), "id")
	if len(long) > len("dockan-")+40+len("-id") {
		t.Fatalf("unit name not bounded: %q", long)
	}
}

func TestProcInScopeReadsOwnCgroup(t *testing.T) {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		t.Skip("no /proc/self/cgroup")
	}
	path := strings.TrimSpace(string(data))
	last := path[strings.LastIndex(path, "/")+1:]
	if !strings.HasSuffix(last, ".scope") {
		t.Skipf("test process not in a scope (%s)", path)
	}
	if !procInScope(os.Getpid(), strings.TrimSuffix(last, ".scope")) {
		t.Fatalf("own scope %s not recognised", last)
	}
	if procInScope(os.Getpid(), "dockan-not-this-one") {
		t.Fatal("foreign scope must not match")
	}
}

// Limits on a scope are read back from systemd, whose output order differs
// from the order the properties are asked for.
func TestConfirmScopeChecksAppliedLimits(t *testing.T) {
	if !systemdScopesAvailable() {
		t.Skip("systemd scopes not available for this user")
	}
	runID, err := newRunID()
	if err != nil {
		t.Fatal(err)
	}
	opts := RunOptions{Memory: "48m", CPUs: "0.5"}
	cmd, unit, err := applyDetachedLaunch(exec.Command("sleep", "72.25"), opts, scopeUnitName("limits", runID))
	if err != nil || unit == "" {
		t.Fatalf("applyDetachedLaunch = %q, %v", unit, err)
	}
	cmd.SysProcAttr = detachedSysProcAttr(false)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill() // end the test workload
		_ = cmd.Wait()
	}()
	if err := confirmScope(cmd.Process.Pid, unit, opts); err != nil {
		t.Fatalf("limits applied by systemd were not recognised: %v", err)
	}
}

// A process that clears its environment and double-forks cannot be found by
// group, parent or run id: the container's cgroup still holds it.
func TestStopFindsWorkloadThroughItsCgroup(t *testing.T) {
	if !systemdScopesAvailable() {
		t.Skip("systemd scopes not available for this user")
	}
	runID, err := newRunID()
	if err != nil {
		t.Fatal(err)
	}
	base := exec.Command("sh", "-c", `(env -i setsid sleep 66.25 &); exec sleep 67.25`)
	cmd, unit, err := applyDetachedLaunch(base, RunOptions{}, scopeUnitName("cgtest", runID))
	if err != nil || unit == "" {
		t.Fatalf("applyDetachedLaunch = %q, %v", unit, err)
	}
	cmd.SysProcAttr = detachedSysProcAttr(false)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	go func() {
		_ = cmd.Wait() // reap the leader; its exit status is irrelevant here
	}()
	t.Cleanup(func() {
		for _, m := range []string{"66.25", "67.25"} {
			for _, pid := range processesRunning(t, m) {
				_ = syscall.Kill(pid, syscall.SIGKILL) // best effort after a failed test
			}
		}
	})
	for deadline := time.Now().Add(5 * time.Second); len(processesRunning(t, "66.25")) == 0 || len(processesRunning(t, "67.25")) == 0; {
		if time.Now().After(deadline) {
			t.Fatal("workload never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
	c := Container{PID: cmd.Process.Pid, PIDStart: recordedStartTime(cmd.Process.Pid), Scope: unit}
	stopAndCheck(t, c, "66.25")
	if left := processesRunning(t, "67.25"); len(left) > 0 {
		t.Fatalf("container leader survived stop: %v", left)
	}
}
