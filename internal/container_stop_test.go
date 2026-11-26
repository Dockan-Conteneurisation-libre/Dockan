package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParseProcStat(t *testing.T) {
	line := "4242 (my (odd) cmd) S 1 4240 4240 0 -1 4194560 0 0 0 0 0 0 0 0 20 0 1 0 98765 0 0"
	st, ok := parseProcStat(line)
	if !ok || st.state != "S" || st.ppid != 1 || st.pgrp != 4240 || st.start != 98765 {
		t.Fatalf("parseProcStat = %+v, %v; want S, ppid 1, pgrp 4240, start 98765", st, ok)
	}
	if _, ok := parseProcStat("garbage"); ok {
		t.Fatal("malformed stat line must not parse")
	}
}

func TestEnvironHasMatchesWholeEntries(t *testing.T) {
	env := []byte("PATH=/bin\x00DOCKAN_RUN_ID=abc\x00HOME=/root\x00")
	if !environHas(env, "DOCKAN_RUN_ID=abc") {
		t.Fatal("exact entry not found")
	}
	if environHas(env, "DOCKAN_RUN_ID=ab") || environHas(env, "RUN_ID=abc") {
		t.Fatal("partial entries must not match")
	}
}

// processesRunning lists live processes whose command line is exactly
// "sleep <marker>".
func processesRunning(t *testing.T, marker string) []int {
	t.Helper()
	procs, err := scanProcs("")
	if err != nil {
		t.Fatalf("scanProcs: %v", err)
	}
	var found []int
	for _, p := range procs {
		data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(p.pid), "cmdline"))
		if err != nil {
			continue
		}
		if strings.TrimRight(strings.ReplaceAll(string(data), "\x00", " "), " ") == "sleep "+marker {
			found = append(found, p.pid)
		}
	}
	return found
}

// startTestContainer starts a shell script the way detached containers are
// started (own session, run id in the environment) and waits until its
// "sleep <marker>" processes are visible.
func startTestContainer(t *testing.T, script, marker string, want int) Container {
	t.Helper()
	runID, err := newRunID()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", script)
	cmd.SysProcAttr = detachedSysProcAttr(false)
	cmd.Env = append(os.Environ(), runIDEnv+"="+runID)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	go func() {
		_ = cmd.Wait() // reap the leader; its exit status is irrelevant here
	}()
	t.Cleanup(func() {
		for _, pid := range processesRunning(t, marker) {
			_ = syscall.Kill(pid, syscall.SIGKILL) // best effort after a failed test
		}
	})
	c := Container{PID: cmd.Process.Pid, PIDStart: recordedStartTime(cmd.Process.Pid), RunID: runID}
	for deadline := time.Now().Add(3 * time.Second); len(processesRunning(t, marker)) < want; {
		if time.Now().After(deadline) {
			t.Fatalf("workload %q never started", marker)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return c
}

func stopAndCheck(t *testing.T, c Container, marker string) {
	t.Helper()
	scope, err := captureContainerScope(c)
	if err != nil || scope == nil {
		t.Fatalf("captureContainerScope = %v, %v", scope, err)
	}
	if err := terminateScope(scope); err != nil {
		t.Fatalf("terminateScope: %v", err)
	}
	if left := processesRunning(t, marker); len(left) > 0 {
		t.Fatalf("container processes survived stop: %v", left)
	}
	if containerAlive(c) {
		t.Fatal("container still reported alive after stop")
	}
}

// A sandbox launcher exits on SIGTERM while the workload ignores it.
func TestStopKillsWorkloadIgnoringSIGTERM(t *testing.T) {
	c := startTestContainer(t, `sh -c "trap '' TERM; exec sleep 61.25" & wait`, "61.25", 1)
	time.Sleep(150 * time.Millisecond) // let the trap be installed before exec
	stopAndCheck(t, c, "61.25")
}

// A workload that left the group but is still a child is stopped.
func TestStopKillsWorkloadThatLeftTheGroup(t *testing.T) {
	c := startTestContainer(t, `setsid sleep 62.25 & wait`, "62.25", 1)
	stopAndCheck(t, c, "62.25")
}

// A double-forked workload is re-parented to init (or a subreaper) before
// stop and shares neither group nor parent with the container: only its
// inherited run id identifies it.
func TestStopKillsDoubleForkedWorkload(t *testing.T) {
	c := startTestContainer(t, `(setsid sleep 63.25 &); exec sleep 64.25`, "63.25", 1)
	for _, pid := range processesRunning(t, "63.25") {
		st, ok := readProcStat(pid)
		if ok && st.ppid == c.PID {
			t.Fatal("test setup: the workload must not be a child of the leader")
		}
	}
	stopAndCheck(t, c, "63.25")
	if left := processesRunning(t, "64.25"); len(left) > 0 {
		t.Fatalf("container leader survived stop: %v", left)
	}
}

// A container whose recorded pid now names another process is not alive.
func TestLeaderReplacedByPidReuse(t *testing.T) {
	self := os.Getpid()
	start := processStartTime(self)
	if start == 0 {
		t.Fatal("start time of the test process unreadable")
	}
	if leaderReplaced(self, start) {
		t.Fatal("same pid and start time must not count as replaced")
	}
	if !leaderReplaced(self, start+1) {
		t.Fatal("a different start time must count as replaced")
	}
	if leaderReplaced(self, 0) {
		t.Fatal("unknown start time (older containers) must not count as replaced")
	}
	if containerAlive(Container{PID: self, PIDStart: start + 1}) {
		t.Fatal("a container whose pid was reused must not be reported alive")
	}
}

// Stopping a container already recorded as stopped by an older Dockan (no
// start time) must not signal whatever process now holds its pid: here, the
// test process itself.
func TestStopNeverSignalsPidOfOldStoppedContainer(t *testing.T) {
	t.Setenv("DOCKAN_HOME", t.TempDir())
	dir := filepath.Join(ContainersDir(), "old")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	meta := map[string]string{"name": "old", "pid": strconv.Itoa(os.Getpid()), "status": "stopped"}
	if err := WriteMeta(filepath.Join(dir, "meta.conf"), meta); err != nil {
		t.Fatal(err)
	}
	if err := StopContainer("old"); err != nil {
		t.Fatalf("StopContainer: %v", err)
	}
	got, err := ParseMeta(filepath.Join(dir, "meta.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if got["status"] != "exited" {
		t.Fatalf("status = %q, want exited", got["status"])
	}
}
