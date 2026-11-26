package internal

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// runIDEnv marks every process of one container run. The variable is
// inherited across fork and exec, so a process that left the group and was
// re-parented to init (double fork) is still recognised.
const runIDEnv = "DOCKAN_RUN_ID"

// newRunID returns a random identifier for one container run.
func newRunID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("identifiant de lancement: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// procInfo is the part of /proc/<pid> that identifies a process and its place
// in a container.
type procInfo struct {
	pid    int
	ppid   int
	pgrp   int
	start  uint64
	pidNS  string
	tagged bool // carries the container's run id in its environment
}

// procStat holds the fields read from /proc/<pid>/stat.
type procStat struct {
	state string
	ppid  int
	pgrp  int
	start uint64
}

// parseProcStat reads state, ppid, pgrp and starttime from a /proc/<pid>/stat
// line. The command name may contain spaces and parentheses, so fields are
// counted after its last closing parenthesis: state is field 3 of the line,
// ppid 4, pgrp 5 and starttime 22.
func parseProcStat(stat string) (procStat, bool) {
	end := strings.LastIndexByte(stat, ')')
	if end < 0 {
		return procStat{}, false
	}
	fields := strings.Fields(stat[end+1:])
	if len(fields) < 20 {
		return procStat{}, false
	}
	ppid, errP := strconv.Atoi(fields[1])
	pgrp, errG := strconv.Atoi(fields[2])
	start, errS := strconv.ParseUint(fields[19], 10, 64)
	if errP != nil || errG != nil || errS != nil {
		return procStat{}, false
	}
	return procStat{state: fields[0], ppid: ppid, pgrp: pgrp, start: start}, true
}

func readProcStat(pid int) (procStat, bool) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return procStat{}, false
	}
	return parseProcStat(string(data))
}

// processStartTime returns the kernel start time of pid, or 0 when the
// process is gone. Together with the pid it names one process for good.
func processStartTime(pid int) uint64 {
	st, ok := readProcStat(pid)
	if !ok {
		return 0
	}
	return st.start
}

// environHas reports whether a NUL-separated environment block holds exactly
// the entry want.
func environHas(environ []byte, want string) bool {
	for _, entry := range bytes.Split(environ, []byte{0}) {
		if string(entry) == want {
			return true
		}
	}
	return false
}

// scanProcs lists live (non-zombie) processes. When marker is not empty, it
// also reports which processes carry it in their environment. Processes that
// exit during the scan, or whose details are not readable for this user, are
// kept with what could be read.
func scanProcs(marker string) ([]procInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	procs := make([]procInfo, 0, len(entries))
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		st, ok := readProcStat(pid)
		if !ok || st.state == "Z" {
			continue
		}
		p := procInfo{pid: pid, ppid: st.ppid, pgrp: st.pgrp, start: st.start}
		if ns, err := os.Readlink(filepath.Join("/proc", entry.Name(), "ns", "pid")); err == nil {
			p.pidNS = ns
		}
		if marker != "" {
			if env, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "environ")); err == nil {
				p.tagged = environHas(env, marker)
			}
		}
		procs = append(procs, p)
	}
	return procs, nil
}

// containerScope identifies every process of one container run: processes
// carrying its run id, its leader's process group, the leader's descendants
// and the PID namespaces they own. It grows with every scan, so a process
// forked while the container stops is not lost when its parent exits.
type containerScope struct {
	leader     int
	marker     string
	namespaces map[string]bool
	members    map[int]uint64 // pid -> start time
	// cutoff is the latest start time seen at capture. A process that only
	// shares the leader's group id and started later can be an unrelated
	// process that reused the id: it is not matched on that ground alone.
	cutoff uint64
}

// captureContainerScope records the container's processes before any signal.
// It returns a nil scope when the recorded leader pid now names another
// process (pid reuse).
func captureContainerScope(c Container) (*containerScope, error) {
	if c.PID <= 0 || leaderReplaced(c.PID, c.PIDStart) {
		return nil, nil
	}
	s := &containerScope{leader: c.PID, namespaces: map[string]bool{}, members: map[int]uint64{}}
	if c.RunID != "" {
		s.marker = runIDEnv + "=" + c.RunID
	}
	procs, err := scanProcs(s.marker)
	if err != nil {
		return nil, err
	}
	for _, p := range procs {
		if p.start > s.cutoff {
			s.cutoff = p.start
		}
	}
	hostNS, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		return nil, fmt.Errorf("espace de noms PID de l'hôte illisible: %w", err)
	}
	for _, p := range s.match(procs, true) {
		s.members[p.pid] = p.start
		if p.pidNS != "" && p.pidNS != hostNS {
			s.namespaces[p.pidNS] = true
		}
	}
	return s, nil
}

// match returns the processes of the container among procs and the
// descendants of each of them. withLeader also includes the leader itself,
// once leaderReplaced has ruled out pid reuse.
func (s *containerScope) match(procs []procInfo, withLeader bool) []procInfo {
	children := make(map[int][]int, len(procs))
	byPID := make(map[int]procInfo, len(procs))
	var queue []int
	in := map[int]bool{}
	for _, p := range procs {
		byPID[p.pid] = p
		children[p.ppid] = append(children[p.ppid], p.pid)
		start, known := s.members[p.pid]
		inGroup := p.pgrp == s.leader && p.start <= s.cutoff
		if p.tagged || inGroup || (known && start == p.start) || s.namespaces[p.pidNS] ||
			(withLeader && p.pid == s.leader) {
			in[p.pid] = true
			queue = append(queue, p.pid)
		}
	}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		for _, child := range children[pid] {
			if !in[child] {
				in[child] = true
				queue = append(queue, child)
			}
		}
	}
	out := make([]procInfo, 0, len(in))
	for pid := range in {
		out = append(out, byPID[pid])
	}
	return out
}

// alive rescans /proc, returns the container's live processes and records
// them as members.
func (s *containerScope) alive() ([]procInfo, error) {
	procs, err := scanProcs(s.marker)
	if err != nil {
		return nil, err
	}
	found := s.match(procs, false)
	for _, p := range found {
		s.members[p.pid] = p.start
	}
	return found, nil
}

// leaderReplaced reports whether the recorded leader pid now names another
// process (pid reuse). Containers started before start times were recorded
// (start == 0) cannot be checked.
func leaderReplaced(leader int, start uint64) bool {
	if start == 0 {
		return false
	}
	current := processStartTime(leader)
	return current != 0 && current != start
}

// containerAlive reports whether any process of the container still runs.
func containerAlive(c Container) bool {
	s, err := captureContainerScope(c)
	if err != nil {
		// /proc unreadable: fall back to the leader and its group.
		return processRunning(c.PID) || syscall.Kill(-c.PID, 0) == nil
	}
	return s != nil && len(s.members) > 0
}

// terminateScope sends SIGTERM to every process of the container, waits up
// to two seconds, then sends SIGKILL until nothing is left or two more
// seconds have passed.
func terminateScope(s *containerScope) error {
	if err := signalContainerScope(s, syscall.SIGTERM); err != nil {
		return err
	}
	if waitScopeExit(s, 2*time.Second) {
		return nil
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := signalContainerScope(s, syscall.SIGKILL); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
		left, err := s.alive()
		if err != nil {
			return err
		}
		if len(left) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%d processus du conteneur toujours actifs après SIGKILL", len(left))
		}
	}
}

// signalContainerScope signals each identified process individually. The
// group id is never signalled as a whole: once the container's group is
// empty, that id could belong to an unrelated process. A failure on one
// process does not stop the others; failures are reported together.
func signalContainerScope(s *containerScope, sig syscall.Signal) error {
	procs, err := s.alive()
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range procs {
		err := syscall.Kill(p.pid, sig)
		switch {
		case err == nil || err == syscall.ESRCH:
		case err == syscall.EPERM:
			errs = append(errs, fmt.Errorf("droits insuffisants pour signaler le processus %d du conteneur: %w", p.pid, err))
		default:
			errs = append(errs, fmt.Errorf("signal au processus %d: %w", p.pid, err))
		}
	}
	return errors.Join(errs...)
}

func waitScopeExit(s *containerScope, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		procs, err := s.alive()
		if err == nil && len(procs) == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// recordedStartTime reads the start time of a just-started process, retrying
// briefly while /proc catches up. It returns 0 when the process already
// exited.
func recordedStartTime(pid int) uint64 {
	for i := 0; i < 5; i++ {
		if start := processStartTime(pid); start != 0 {
			return start
		}
		time.Sleep(10 * time.Millisecond)
	}
	return 0
}
