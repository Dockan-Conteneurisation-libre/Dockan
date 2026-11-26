package internal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Each detached container runs in its own systemd scope, which is a cgroup
// of its own. A process cannot leave a cgroup by forking, by changing its
// session or by clearing its environment, so "dockan stop" finds every
// process of the container in it. When systemd scopes are not available
// (no systemd, no user manager), Dockan falls back to the other ways of
// identifying the container's processes.

var scopeSupport struct {
	once sync.Once
	ok   bool
}

func systemdScopeArgs() []string {
	args := []string{"--quiet", "--scope", "--collect"}
	if os.Geteuid() != 0 {
		args = append([]string{"--user"}, args...)
	}
	return args
}

// systemdScopesAvailable checks once that systemd-run can create a scope for
// this user. For a regular user the scope lives in the user's systemd
// manager, which stops with the last login session unless "linger" is
// enabled: without linger, a background container would be killed at logout,
// so scopes are only used when linger is on.
func systemdScopesAvailable() bool {
	scopeSupport.once.Do(func() {
		if !commandExists("systemd-run") {
			return
		}
		if os.Geteuid() != 0 && !userLingerEnabled() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		args := append(systemdScopeArgs(), "--", "true")
		scopeSupport.ok = exec.CommandContext(ctx, "systemd-run", args...).Run() == nil
	})
	return scopeSupport.ok
}

func userLingerEnabled() bool {
	if !commandExists("loginctl") {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "loginctl", "show-user", strconv.Itoa(os.Getuid()), "-p", "Linger", "--value").Output()
	return err == nil && strings.TrimSpace(string(out)) == "yes"
}

func systemctlScopeArgs(unit string, props ...string) []string {
	args := []string{"show", unit + ".scope"}
	for _, p := range props {
		args = append(args, "-p", p)
	}
	if os.Geteuid() != 0 {
		args = append([]string{"--user"}, args...)
	}
	return args
}

// confirmScope waits until the container's leader runs inside its scope and
// checks that the requested limits are really applied (a user manager may
// lack the memory or cpu controller and silently ignore them).
func confirmScope(pid int, unit string, opts RunOptions) error {
	deadline := time.Now().Add(3 * time.Second)
	for !procInScope(pid, unit) {
		if !processRunning(pid) {
			return fmt.Errorf("le conteneur s'est arrêté au démarrage (voir dockan logs)")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("le conteneur n'a pas rejoint son scope systemd %s", unit)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if opts.Memory == "" && opts.CPUs == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", systemctlScopeArgs(unit, "MemoryMax", "CPUQuotaPerSecUSec")...).Output()
	if err != nil {
		return fmt.Errorf("lecture des limites du scope %s: %w", unit, err)
	}
	// "systemctl show" prints Key=Value lines in its own order, not in the
	// order the properties were asked for.
	values := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			values[key] = strings.TrimSpace(value)
		}
	}
	if opts.Memory != "" {
		want, err := parseMemoryBytes(opts.Memory)
		if err != nil {
			return err
		}
		if values["MemoryMax"] != strconv.FormatInt(want, 10) {
			return fmt.Errorf("--memory non appliqué: le contrôleur memory n'est pas disponible pour ce scope")
		}
	}
	if opts.CPUs != "" && (values["CPUQuotaPerSecUSec"] == "" || values["CPUQuotaPerSecUSec"] == "infinity") {
		return fmt.Errorf("--cpus non appliqué: le contrôleur cpu n'est pas disponible pour ce scope")
	}
	return nil
}

// scopeUnitName returns a systemd unit name unique to one container run.
func scopeUnitName(name, runID string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	safe := b.String()
	if len(safe) > 40 {
		safe = safe[:40]
	}
	id := runID
	if len(id) > 12 {
		id = id[:12]
	}
	return "dockan-" + safe + "-" + id
}

// applyDetachedLaunch places the container in its own systemd scope, with
// the memory and CPU limits on that same scope. Without systemd scopes it
// keeps the previous behaviour (limits only) and returns an empty unit.
func applyDetachedLaunch(cmd *exec.Cmd, opts RunOptions, unit string) (*exec.Cmd, string, error) {
	if !systemdScopesAvailable() {
		wrapped, err := ApplyRunLimits(cmd, opts)
		return wrapped, "", err
	}
	props, err := systemdLimitProperties(opts)
	if err != nil {
		return nil, "", err
	}
	args := append(systemdScopeArgs(), "--unit="+unit)
	args = append(args, props...)
	args = append(args, "--")
	args = append(args, cmd.Args...)
	wrapped := exec.Command("systemd-run", args...)
	wrapped.Dir = cmd.Dir
	wrapped.SysProcAttr = cmd.SysProcAttr
	return wrapped, unit, nil
}

// lastLogLines returns the last n lines of a log file, or "" when it cannot
// be read.
func lastLogLines(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// procInScope reports whether pid belongs to the cgroup of the given scope
// unit (or to a cgroup below it).
func procInScope(pid int, unit string) bool {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return false
	}
	want := unit + ".scope"
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		for _, elem := range strings.Split(parts[2], "/") {
			if elem == want {
				return true
			}
		}
	}
	return false
}
