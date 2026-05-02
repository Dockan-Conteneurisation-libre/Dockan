package internal

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func ApplyRunLimits(cmd *exec.Cmd, opts RunOptions) (*exec.Cmd, error) {
	if opts.Memory == "" && opts.CPUs == "" {
		return cmd, nil
	}
	if commandExists("systemd-run") {
		wrapped, err := applySystemdRunLimits(cmd, opts)
		if err == nil {
			return wrapped, nil
		}
		if opts.CPUs != "" {
			return nil, err
		}
	}
	if opts.CPUs != "" {
		return nil, fmt.Errorf("--cpus nécessite systemd-run pour appliquer une limite cgroup CPU")
	}
	return applyPrlimitMemory(cmd, opts.Memory)
}

func applyPrlimitMemory(cmd *exec.Cmd, memory string) (*exec.Cmd, error) {
	bytes, err := parseMemoryBytes(memory)
	if err != nil {
		return nil, err
	}
	if !commandExists("prlimit") {
		return nil, fmt.Errorf("--memory nécessite prlimit ou systemd-run sur cette machine")
	}
	args := []string{"--as=" + strconv.FormatInt(bytes, 10), "--"}
	args = append(args, cmd.Args...)
	wrapped := exec.Command("prlimit", args...)
	wrapped.Dir = cmd.Dir
	wrapped.SysProcAttr = cmd.SysProcAttr
	return wrapped, nil
}

func applySystemdRunLimits(cmd *exec.Cmd, opts RunOptions) (*exec.Cmd, error) {
	args := []string{"--quiet", "--scope"}
	if opts.Memory != "" {
		bytes, err := parseMemoryBytes(opts.Memory)
		if err != nil {
			return nil, err
		}
		args = append(args, "-p", "MemoryMax="+strconv.FormatInt(bytes, 10))
	}
	if opts.CPUs != "" {
		quota, err := cpuQuotaPercent(opts.CPUs)
		if err != nil {
			return nil, err
		}
		args = append(args, "-p", "CPUQuota="+quota)
	}
	args = append(args, "--")
	args = append(args, cmd.Args...)
	wrapped := exec.Command("systemd-run", args...)
	wrapped.Dir = cmd.Dir
	wrapped.SysProcAttr = cmd.SysProcAttr
	return wrapped, nil
}

func cpuQuotaPercent(value string) (string, error) {
	cpus, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || cpus <= 0 {
		return "", fmt.Errorf("cpus invalide: %s", value)
	}
	return strconv.FormatFloat(cpus*100, 'f', -1, 64) + "%", nil
}

func parseMemoryBytes(value string) (int64, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 0, fmt.Errorf("mémoire vide")
	}
	multiplier := int64(1)
	for _, suffix := range []struct {
		name string
		mul  int64
	}{
		{"gb", 1024 * 1024 * 1024},
		{"g", 1024 * 1024 * 1024},
		{"mb", 1024 * 1024},
		{"m", 1024 * 1024},
		{"kb", 1024},
		{"k", 1024},
	} {
		if strings.HasSuffix(value, suffix.name) {
			multiplier = suffix.mul
			value = strings.TrimSuffix(value, suffix.name)
			break
		}
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("mémoire invalide: %s", value)
	}
	return int64(number * float64(multiplier)), nil
}

func validateCPUs(value string) error {
	if value == "" {
		return nil
	}
	cpus, err := strconv.ParseFloat(value, 64)
	if err != nil || cpus <= 0 {
		return fmt.Errorf("cpus invalide: %s", value)
	}
	return nil
}
