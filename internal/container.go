package internal

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Container struct {
	Name    string
	Image   string
	Path    string
	PID     int
	Status  string
	Ports   string
	Network string
	Created string
}

func ContainersDir() string {
	return filepath.Join(StoreRoot(), "containers")
}

func containersDirForRoot(root string) string {
	return filepath.Join(root, "containers")
}

func StartDetachedContainer(imagePath, imageRef string, opts RunOptions) error {
	img, err := LoadImage(imagePath)
	if err != nil {
		return err
	}
	if err := RepairOCIRootfs(img); err != nil {
		return err
	}
	if opts.Name == "" {
		opts.Name = generatedContainerName(img)
	}
	isolation, err := ResolveIsolation(opts.Isolation)
	if err != nil {
		return err
	}
	if err := ValidateRunOptions(opts); err != nil {
		return err
	}
	containerDir := filepath.Join(ContainersDir(), opts.Name)
	if _, err := os.Stat(containerDir); err == nil {
		return fmt.Errorf("conteneur déjà existant: %s", opts.Name)
	}
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		return err
	}
	runtimeVolumes := EffectiveRunVolumes(opts)
	var volumeBinds []VolumeBind
	if usesPrivateBubblewrapBinds(isolation, img) {
		volumeBinds, err = PrepareVolumeBindsForRun(imagePath, img.Meta, runtimeVolumes)
	} else {
		_, err = PrepareVolumesForRun(imagePath, img.Meta, runtimeVolumes)
	}
	if err != nil {
		return err
	}
	networkMeta, bridgeNetwork, err := prepareDetachedNetwork(opts.Network)
	if err != nil {
		return err
	}
	cmd, err := isolationCommand(isolation, img, volumeBinds)
	if err != nil {
		return err
	}
	cmd, err = ApplyRunLimits(cmd, opts)
	if err != nil {
		return err
	}
	networkReadyPath := ""
	if bridgeNetwork {
		networkReadyPath = filepath.Join(containerDir, "network-ready")
		_ = os.Remove(networkReadyPath)
		cmd = gateCommandUntilFile(cmd, networkReadyPath)
	}
	logFile, err := os.OpenFile(filepath.Join(containerDir, "dockan.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.Env = imageEnv(img, opts)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if bridgeNetwork {
		cmd.SysProcAttr.Cloneflags = syscall.CLONE_NEWNET
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	networkAttachment := NetworkAttachment{}
	if bridgeNetwork {
		networkAttachment, err = AttachBridgeNetwork(opts.Name, cmd.Process.Pid, networkMeta)
		if err != nil {
			_ = cmd.Process.Kill()
			_ = os.RemoveAll(containerDir)
			return err
		}
		if err := os.WriteFile(networkReadyPath, []byte("ready\n"), 0644); err != nil {
			_ = cmd.Process.Kill()
			_ = CleanupContainerNetwork(map[string]string{"vethHost": networkAttachment.HostInterface})
			_ = os.RemoveAll(containerDir)
			return err
		}
	}
	var portProxyPIDs []int
	if bridgeNetwork && networkAttachment.IP != "" && len(opts.Ports) > 0 {
		portProxyPIDs, err = StartPortProxies(containerDir, opts.Ports, networkAttachment.IP)
		if err != nil {
			_ = cmd.Process.Kill()
			_ = CleanupContainerNetwork(map[string]string{"vethHost": networkAttachment.HostInterface})
			_ = os.RemoveAll(containerDir)
			return err
		}
	} else if len(opts.Ports) > 0 {
		hostSharedProxyPorts := hostSharedProxyPorts(opts.Ports)
		if len(hostSharedProxyPorts) > 0 {
			portProxyPIDs, err = StartPortProxies(containerDir, hostSharedProxyPorts, "127.0.0.1")
			if err != nil {
				_ = cmd.Process.Kill()
				_ = os.RemoveAll(containerDir)
				return err
			}
		}
	}
	meta := map[string]string{
		"name":       opts.Name,
		"image":      imageRef,
		"imagePath":  imagePath,
		"pid":        strconv.Itoa(cmd.Process.Pid),
		"status":     "running",
		"isolation":  isolation,
		"ports":      strings.Join(opts.Ports, ","),
		"network":    opts.Network,
		"aliases":    strings.Join(opts.Aliases, ","),
		"volumes":    strings.Join(opts.Volumes, ","),
		"gui":        fmt.Sprint(opts.GUI),
		"command":    strings.Join(opts.Command, " "),
		"entrypoint": opts.Entrypoint,
		"restart":    opts.Restart,
		"memory":     opts.Memory,
		"cpus":       opts.CPUs,
		"created":    time.Now().Format(time.RFC3339),
	}
	if healthcheck := strings.TrimSpace(opts.Healthcheck); healthcheck != "" {
		meta["healthcheck"] = healthcheck
	} else if healthcheck := strings.TrimSpace(img.Meta["healthcheck"]); healthcheck != "" {
		meta["healthcheck"] = healthcheck
	}
	if len(portProxyPIDs) > 0 {
		meta["portProxyPids"] = joinInts(portProxyPIDs)
	}
	if networkAttachment.IP != "" {
		meta["networkIP"] = networkAttachment.IP
		meta["vethHost"] = networkAttachment.HostInterface
		meta["vethContainer"] = networkAttachment.ContainerInterface
	}
	if err := WriteMeta(filepath.Join(containerDir, "meta.conf"), meta); err != nil {
		_ = CleanupPIDs(portProxyPIDs)
		_ = cmd.Process.Kill()
		_ = CleanupContainerNetwork(meta)
		return err
	}
	if opts.Network != "" && opts.Network != HostNetwork {
		_ = WriteNetworkHosts(opts.Network)
	}
	_ = cmd.Process.Release()
	fmt.Printf("%s\n", opts.Name)
	return nil
}

func gateCommandUntilFile(cmd *exec.Cmd, readyPath string) *exec.Cmd {
	args := []string{
		"-c",
		`ready=$1; shift; while [ ! -e "$ready" ]; do sleep 0.05 2>/dev/null || sleep 1; done; exec "$@"`,
		"dockan-network-gate",
		readyPath,
	}
	args = append(args, cmd.Args...)
	gated := exec.Command("sh", args...)
	gated.Dir = cmd.Dir
	return gated
}

func hostSharedProxyPorts(ports []string) []string {
	var proxyPorts []string
	for _, published := range ports {
		hostPort, containerPort, err := splitPublishedPort(published)
		if err != nil {
			continue
		}
		if hostPort != containerPort {
			proxyPorts = append(proxyPorts, published)
		}
	}
	return proxyPorts
}

func ListContainers(all bool) error {
	return ListContainersFromRoot(all, StoreRoot())
}

func ListContainersFromRoot(all bool, root string) error {
	containers, err := LoadContainersFromRoot(root)
	if err != nil {
		return err
	}
	fmt.Printf("%-18s %-12s %-8s %-24s %s\n", "NAME", "STATUS", "PID", "IMAGE", "PORTS")
	for _, c := range containers {
		if !all && c.Status != "running" {
			continue
		}
		fmt.Printf("%-18s %-12s %-8d %-24s %s\n", c.Name, c.Status, c.PID, c.Image, c.Ports)
	}
	return nil
}

func ListContainersFromScopes(all bool, scopes []StoreScope) error {
	if len(scopes) == 1 {
		return ListContainersFromRoot(all, scopes[0].Root)
	}
	fmt.Printf("%-10s %-18s %-12s %-8s %-24s %s\n", "STORE", "NAME", "STATUS", "PID", "IMAGE", "PORTS")
	for _, scope := range scopes {
		containers, err := LoadContainersFromRoot(scope.Root)
		if os.IsPermission(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, c := range containers {
			if !all && c.Status != "running" {
				continue
			}
			fmt.Printf("%-10s %-18s %-12s %-8d %-24s %s\n", scope.Label, c.Name, c.Status, c.PID, c.Image, c.Ports)
		}
	}
	return nil
}

func LoadContainers() ([]Container, error) {
	return LoadContainersFromRoot(StoreRoot())
}

func LoadContainersFromRoot(root string) ([]Container, error) {
	dir := containersDirForRoot(root)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var containers []Container
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		containerDir := filepath.Join(dir, entry.Name())
		meta, err := ParseMeta(filepath.Join(containerDir, "meta.conf"))
		if err != nil {
			continue
		}
		pid, _ := strconv.Atoi(meta["pid"])
		status := meta["status"]
		if pid > 0 && !processRunning(pid) && status == "running" {
			status = "exited"
			meta["status"] = status
			_ = CleanupPortProxies(meta)
			_ = CleanupContainerNetwork(meta)
			_ = WriteMeta(filepath.Join(containerDir, "meta.conf"), meta)
		}
		containers = append(containers, Container{
			Name:    entry.Name(),
			Image:   meta["image"],
			Path:    containerDir,
			PID:     pid,
			Status:  status,
			Ports:   meta["ports"],
			Network: meta["network"],
			Created: meta["created"],
		})
	}
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Name < containers[j].Name
	})
	return containers, nil
}

func StopContainer(name string) error {
	c, meta, err := loadContainer(name)
	if err != nil {
		return err
	}
	if c.PID <= 0 || !processRunning(c.PID) {
		meta["status"] = "exited"
		_ = CleanupPortProxies(meta)
		_ = CleanupContainerNetwork(meta)
		return WriteMeta(filepath.Join(c.Path, "meta.conf"), meta)
	}
	if err := signalContainerProcess(c.PID, syscall.SIGTERM); err != nil {
		return err
	}
	for i := 0; i < 20 && processRunning(c.PID); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if processRunning(c.PID) {
		_ = signalContainerProcess(c.PID, syscall.SIGKILL)
		for i := 0; i < 20 && processRunning(c.PID); i++ {
			time.Sleep(100 * time.Millisecond)
		}
	}
	meta["status"] = "stopped"
	_ = CleanupPortProxies(meta)
	_ = CleanupContainerNetwork(meta)
	if err := WriteMeta(filepath.Join(c.Path, "meta.conf"), meta); err != nil {
		return err
	}
	if c.Network != "" && c.Network != HostNetwork {
		_ = WriteNetworkHosts(c.Network)
	}
	return nil
}

func RemoveContainer(name string) error {
	c, _, err := loadContainer(name)
	if err != nil {
		return err
	}
	if c.PID > 0 && processRunning(c.PID) {
		return fmt.Errorf("conteneur en cours: stoppez-le d'abord")
	}
	meta, _ := ParseMeta(filepath.Join(c.Path, "meta.conf"))
	_ = CleanupPortProxies(meta)
	_ = CleanupContainerNetwork(meta)
	if imagePath := meta["imagePath"]; imagePath != "" {
		_ = CleanupImageMounts(imagePath)
	}
	if err := os.RemoveAll(c.Path); err != nil {
		return err
	}
	if c.Network != "" && c.Network != HostNetwork {
		_ = WriteNetworkHosts(c.Network)
	}
	return nil
}

func PrintContainerLogs(name string) error {
	c, _, err := loadContainer(name)
	if err != nil {
		return err
	}
	f, err := os.Open(filepath.Join(c.Path, "dockan.log"))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(os.Stdout, f)
	return err
}

func InspectContainer(name string) error {
	c, meta, err := loadContainer(name)
	if err != nil {
		return err
	}
	if c.PID > 0 && !processRunning(c.PID) && meta["status"] == "running" {
		meta["status"] = "exited"
	}
	var keys []string
	for key := range meta {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("%s=%s\n", key, meta[key])
	}
	return nil
}

func ExecContainer(name string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("Usage: dockan exec <conteneur> <commande> [args...]")
	}
	c, meta, err := loadContainer(name)
	if err != nil {
		return err
	}
	if c.PID <= 0 || !processRunning(c.PID) {
		return fmt.Errorf("conteneur non démarré: %s", name)
	}
	if commandExists("nsenter") {
		cmdArgs := append([]string{"-t", strconv.Itoa(c.PID), "-m", "-u", "-i", "-n", "-p", "--"}, args...)
		cmd := exec.Command("nsenter", cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err == nil {
			return nil
		}
		fmt.Fprintln(os.Stderr, "[dockan] nsenter impossible; exécution dans le contexte image")
	}
	imagePath := meta["imagePath"]
	if imagePath == "" {
		imagePath = c.Image
	}
	img, err := LoadImage(imagePath)
	if err != nil {
		return err
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = img.Path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = imageEnv(img, RunOptions{Network: c.Network, Ports: splitComma(c.Ports), Volumes: splitComma(meta["volumes"]), GUI: meta["gui"] == "true"})
	return cmd.Run()
}

func CheckContainerHealth(name string) error {
	c, meta, err := loadContainer(name)
	if err != nil {
		return err
	}
	if c.PID <= 0 || !processRunning(c.PID) {
		return fmt.Errorf("conteneur non démarré: %s", name)
	}
	check, err := healthcheckCommand(meta["healthcheck"])
	if err != nil {
		return err
	}
	if check == "" {
		return fmt.Errorf("aucun healthcheck défini: %s", name)
	}
	if err := ExecContainer(name, []string{"sh", "-lc", check}); err != nil {
		if fallback := bridgeHealthcheckFallback(check, meta); fallback != "" {
			cmd := exec.Command("sh", "-lc", fallback)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if fallbackErr := cmd.Run(); fallbackErr == nil {
				fmt.Printf("%s healthy\n", name)
				return nil
			}
		}
		return fmt.Errorf("%s unhealthy: %w", name, err)
	}
	fmt.Printf("%s healthy\n", name)
	return nil
}

func bridgeHealthcheckFallback(check string, meta map[string]string) string {
	ip := strings.Split(strings.TrimSpace(meta["networkIP"]), "/")[0]
	if ip == "" {
		return ""
	}
	rewritten := strings.ReplaceAll(check, "http://127.0.0.1", "http://"+ip)
	rewritten = strings.ReplaceAll(rewritten, "http://localhost", "http://"+ip)
	rewritten = strings.ReplaceAll(rewritten, "https://127.0.0.1", "https://"+ip)
	rewritten = strings.ReplaceAll(rewritten, "https://localhost", "https://"+ip)
	if rewritten == check {
		return ""
	}
	return rewritten
}

func healthcheckCommand(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.EqualFold(value, "NONE") {
		return "", nil
	}
	fields := strings.Fields(value)
	for len(fields) > 0 && strings.HasPrefix(fields[0], "--") {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return "", nil
	}
	kind := strings.ToUpper(fields[0])
	rest := strings.TrimSpace(strings.TrimPrefix(valueAfterOptions(value), fields[0]))
	switch kind {
	case "CMD":
		return normalizeDockerCommand(rest)
	case "CMD-SHELL":
		return rest, nil
	default:
		return normalizeDockerCommand(value)
	}
}

func valueAfterOptions(value string) string {
	value = strings.TrimSpace(value)
	for {
		fields := strings.Fields(value)
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "--") {
			return value
		}
		value = strings.TrimSpace(strings.TrimPrefix(value, fields[0]))
	}
}

func loadContainer(name string) (Container, map[string]string, error) {
	containerDir := filepath.Join(ContainersDir(), name)
	meta, err := ParseMeta(filepath.Join(containerDir, "meta.conf"))
	if err != nil {
		return Container{}, nil, fmt.Errorf("conteneur introuvable: %s", name)
	}
	pid, _ := strconv.Atoi(meta["pid"])
	return Container{
		Name:    name,
		Image:   meta["image"],
		Path:    containerDir,
		PID:     pid,
		Status:  meta["status"],
		Ports:   meta["ports"],
		Network: meta["network"],
		Created: meta["created"],
	}, meta, nil
}

func splitComma(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err != nil {
		return false
	}
	if data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat")); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 2 && fields[2] == "Z" {
			return false
		}
	}
	return true
}

func signalContainerProcess(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return nil
	}
	if err := syscall.Kill(-pid, sig); err == nil {
		return nil
	}
	return syscall.Kill(pid, sig)
}

func generatedContainerName(img *Image) string {
	base := img.Meta["name"]
	if base == "" {
		base = "dockan"
	}
	return safeTag(base) + "-" + strconv.FormatInt(time.Now().Unix(), 36)
}

func joinInts(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ",")
}
