package internal

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Image struct {
	Path        string
	Meta        map[string]string
	RootfsDir   string
	StartScript string
}

type RunOptions struct {
	Isolation   string
	Detach      bool
	Name        string
	Env         []string
	Ports       []string
	Network     string
	Aliases     []string
	Volumes     []string
	GUI         bool
	Command     []string
	Entrypoint  string
	Restart     string
	Healthcheck string
	Memory      string
	CPUs        string
}

func DefaultRunOptions() RunOptions {
	return RunOptions{Isolation: IsolationAuto}
}

func LoadImage(path string) (*Image, error) {
	meta, err := ParseMeta(filepath.Join(path, "meta.conf"))
	if err != nil {
		return nil, err
	}
	rootfs := filepath.Join(path, "rootfs")
	if _, err := os.Stat(rootfs); err != nil {
		return nil, fmt.Errorf("rootfs/ manquant dans %s", path)
	}
	startScript := filepath.Join(path, "start.sh")
	if _, err := os.Stat(startScript); err != nil {
		return nil, fmt.Errorf("start.sh manquant dans %s", path)
	}
	return &Image{
		Path:        path,
		Meta:        meta,
		RootfsDir:   rootfs,
		StartScript: startScript,
	}, nil
}

func RunImage(path string, opts RunOptions) error {
	img, err := LoadImage(path)
	if err != nil {
		return err
	}
	if err := RepairOCIRootfs(img); err != nil {
		return err
	}
	isolation, err := ResolveIsolation(opts.Isolation)
	if err != nil {
		return err
	}
	fmt.Printf("[dockan] Isolation utilisée : %s\n", isolation)
	if isolation == IsolationNone {
		fmt.Println("[dockan] Avertissement : exécution sans isolation forte")
	}
	stdout, stderr, closeLog, err := LogWriters(img.Path)
	if err != nil {
		return err
	}
	defer closeLog()
	return RunWithIsolationWithLogs(isolation, img, opts, stdout, stderr)
}

func RunWithIsolationWithLogs(isolation string, img *Image, opts RunOptions, stdout, stderr io.Writer) error {
	cmd, err := isolationCommand(isolation, img, nil)
	if err != nil {
		return err
	}
	cmd, err = ApplyRunLimits(cmd, opts)
	if err != nil {
		return err
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	cmd.Env = imageEnv(img, opts)
	return cmd.Run()
}

func BuildImage(path string) error {
	buildScript := filepath.Join(path, "build.sh")
	if _, err := os.Stat(buildScript); err != nil {
		return fmt.Errorf("build.sh manquant dans %s", path)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	cmd := exec.Command("bash", "./build.sh")
	cmd.Dir = absPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "DOCKAN_IMAGE_PATH="+absPath, "DOCKAN_ROOTFS="+filepath.Join(absPath, "rootfs"))
	return cmd.Run()
}

func ListImages(base string) {
	filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() && filepath.Ext(path) == ".dockan" {
			meta := filepath.Join(path, "meta.conf")
			name := filepath.Base(path)
			if _, err := os.Stat(meta); err == nil {
				m, _ := ParseMeta(meta)
				if n, ok := m["name"]; ok {
					name = n
				}
			}
			fmt.Printf("%s : %s\n", name, path)
		}
		return nil
	})
}

func InitImage(dir string) error {
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s existe déjà", dir)
	}
	for _, subdir := range []string{"rootfs", "hooks", "volumes"} {
		if err := os.MkdirAll(filepath.Join(dir, subdir), 0755); err != nil {
			return err
		}
	}
	write := func(p, c string, mode os.FileMode) error {
		return os.WriteFile(filepath.Join(dir, p), []byte(c), mode)
	}
	if err := write("build.sh", "#!/usr/bin/env bash\nset -euo pipefail\necho '(build.sh) Ajoute tes instructions ici.'\n", 0755); err != nil {
		return err
	}
	if err := write("start.sh", "#!/usr/bin/env bash\nset -euo pipefail\necho '(start.sh) Hello depuis Dockan !'\necho \"rootfs: ${DOCKAN_ROOTFS:-rootfs}\"\n", 0755); err != nil {
		return err
	}
	return write("meta.conf", "# Métadonnées Dockan\nname=MonApp\nport=8080\nrequires=bash\n", 0644)
}

func imageEnv(img *Image, opts RunOptions) []string {
	env := cleanHostEnvForContainer(os.Environ())
	absImage, _ := filepath.Abs(img.Path)
	absRootfs, _ := filepath.Abs(img.RootfsDir)
	env = append(env,
		"DOCKAN_IMAGE_PATH="+absImage,
		"DOCKAN_ROOTFS="+absRootfs,
	)
	for key, value := range img.Meta {
		envKey := "DOCKAN_META_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
		env = append(env, envKey+"="+value)
	}
	for name, host := range VolumeEnv(img.Path, img.Meta, EffectiveRunVolumes(opts)) {
		env = append(env, name+"="+host)
	}
	env = append(env, opts.Env...)
	if len(opts.Command) > 0 {
		env = append(env, "DOCKAN_RUN_COMMAND="+strings.Join(opts.Command, " "))
	}
	if opts.Entrypoint != "" {
		env = append(env, "DOCKAN_ENTRYPOINT="+opts.Entrypoint)
	}
	if opts.Restart != "" {
		env = append(env, "DOCKAN_RESTART="+opts.Restart)
	}
	if opts.Memory != "" {
		env = append(env, "DOCKAN_MEMORY="+opts.Memory)
	}
	if opts.CPUs != "" {
		env = append(env, "DOCKAN_CPUS="+opts.CPUs)
	}
	if opts.GUI {
		env = append(env, "DOCKAN_GUI=1")
		for _, key := range []string{"DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "PULSE_SERVER", "DBUS_SESSION_BUS_ADDRESS"} {
			if value := os.Getenv(key); value != "" {
				env = append(env, key+"="+value)
			}
		}
	}
	if opts.Network != "" {
		env = append(env, "DOCKAN_NETWORK="+opts.Network)
	}
	if len(opts.Ports) > 0 {
		env = append(env, "DOCKAN_PORTS="+strings.Join(opts.Ports, ","))
		if !hasEnvKey(opts.Env, "PORT") {
			if port := portEnvValue(opts); port != "" {
				env = append(env, "PORT="+port)
			}
		}
	}
	return env
}

func cleanHostEnvForContainer(hostEnv []string) []string {
	out := make([]string, 0, len(hostEnv))
	for _, item := range hostEnv {
		key, _, _ := strings.Cut(item, "=")
		if key == "DOCKAN_RUN_COMMAND" ||
			key == "DOCKAN_ENTRYPOINT" ||
			key == "DOCKAN_RESTART" ||
			key == "DOCKAN_MEMORY" ||
			key == "DOCKAN_CPUS" ||
			key == "DOCKAN_IMAGE_PATH" ||
			key == "DOCKAN_ROOTFS" ||
			strings.HasPrefix(key, "DOCKAN_META_") ||
			strings.HasPrefix(key, "DOCKAN_VOLUME_") {
			continue
		}
		out = append(out, item)
	}
	return out
}

func portEnvValue(opts RunOptions) string {
	if len(opts.Ports) == 0 {
		return ""
	}
	hostPort, containerPort, err := splitPublishedPort(opts.Ports[0])
	if err != nil {
		return ""
	}
	if opts.Network != "" && IsBridgeNetwork(opts.Network) {
		return containerPort
	}
	return hostPort
}

func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

func ParseMeta(path string) (map[string]string, error) {
	meta := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return meta, err
	}
	for _, l := range splitLines(string(data)) {
		l = strings.TrimSpace(l)
		if len(l) == 0 || l[0] == '#' {
			continue
		}
		if i := indexOf(l, '='); i > 0 {
			k := strings.TrimSpace(l[:i])
			v := strings.TrimSpace(l[i+1:])
			meta[k] = v
		}
	}
	return meta, nil
}

func WriteMeta(path string, meta map[string]string) error {
	var keys []string
	for key := range meta {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Métadonnées Dockan\n")
	for _, key := range keys {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(meta[key])
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := range s {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
