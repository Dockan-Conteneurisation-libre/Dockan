package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func PrepareVolumes(imagePath string, meta map[string]string) (func(), error) {
	return PrepareVolumesForRun(imagePath, meta, nil)
}

func PrepareVolumesForRun(imagePath string, meta map[string]string, runtimeVolumes []string) (func(), error) {
	var mounted []string
	cleanup := func() {
		for i := len(mounted) - 1; i >= 0; i-- {
			_ = exec.Command("umount", mounted[i]).Run()
		}
	}

	specs := imageVolumeSpecs(meta)
	specs = append(specs, runtimeVolumes...)
	if len(specs) == 0 {
		return cleanup, nil
	}

	for _, v := range specs {
		source, containerPath, err := resolveVolumeSource(imagePath, v, isRuntimeVolume(runtimeVolumes, v))
		if err != nil {
			cleanup()
			return cleanup, err
		}
		cleanContainer, err := cleanVolumeTarget(containerPath)
		if err != nil {
			cleanup()
			return cleanup, err
		}
		host := source
		container := filepath.Join(imagePath, "rootfs", cleanContainer)
		if err := os.MkdirAll(host, 0755); err != nil {
			cleanup()
			return cleanup, err
		}
		if os.Geteuid() == 0 {
			if err := os.MkdirAll(container, 0755); err != nil {
				cleanup()
				return cleanup, err
			}
			fmt.Printf("[dockan] Montage volume %s -> %s\n", host, container)
			if err := bindMount(host, container); err != nil {
				cleanup()
				return cleanup, err
			}
			mounted = append(mounted, container)
			continue
		}
		if err := ensureVolumeLink(host, container); err != nil {
			cleanup()
			return cleanup, err
		}
		fmt.Printf("[dockan] Volume %s disponible via %s\n", host, container)
	}

	return cleanup, nil
}

func EffectiveRunVolumes(opts RunOptions) []string {
	volumes := append([]string{}, opts.Volumes...)
	if !opts.GUI {
		return volumes
	}
	if _, err := os.Stat("/tmp/.X11-unix"); err == nil {
		volumes = append(volumes, "/tmp/.X11-unix:/tmp/.X11-unix:ro")
	}
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		if wayland := os.Getenv("WAYLAND_DISPLAY"); wayland != "" {
			socket := filepath.Join(runtimeDir, wayland)
			if _, err := os.Stat(socket); err == nil {
				volumes = append(volumes, socket+":"+socket)
			}
		}
		if pulse := filepath.Join(runtimeDir, "pulse"); dirExists(pulse) {
			volumes = append(volumes, pulse+":"+pulse)
		}
		if bus := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); strings.HasPrefix(bus, "unix:path=") {
			socket := strings.TrimPrefix(bus, "unix:path=")
			if _, err := os.Stat(socket); err == nil {
				volumes = append(volumes, socket+":"+socket)
			}
		}
	}
	if _, err := os.Stat("/dev/dri"); err == nil {
		volumes = append(volumes, "/dev/dri:/dev/dri")
	}
	return volumes
}

func MountVolumes(imagePath string, meta map[string]string) error {
	_, err := PrepareVolumes(imagePath, meta)
	return err
}

func bindMount(source, target string) error {
	return execCommand("mount", "--bind", source, target)
}

func ensureVolumeLink(source, target string) error {
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			current, err := os.Readlink(target)
			if err == nil && current == source {
				return nil
			}
			return fmt.Errorf("volume cible déjà liée ailleurs: %s", target)
		}
		if info.IsDir() {
			entries, err := os.ReadDir(target)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				if err := os.Remove(target); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("volume cible déjà existante et non vide: %s", target)
			}
		} else {
			return fmt.Errorf("volume cible déjà existante: %s", target)
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	absSource, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	return os.Symlink(absSource, target)
}

func cleanVolumeTarget(target string) (string, error) {
	clean := filepath.Clean(strings.TrimPrefix(target, string(filepath.Separator)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("chemin de volume interdit hors rootfs: %s", target)
	}
	return clean, nil
}

func VolumeEnv(imagePath string, meta map[string]string, runtimeVolumes []string) map[string]string {
	env := make(map[string]string)
	specs := imageVolumeSpecs(meta)
	specs = append(specs, runtimeVolumes...)
	for _, v := range specs {
		source, _, err := resolveVolumeSource(imagePath, v, isRuntimeVolume(runtimeVolumes, v))
		if err != nil {
			continue
		}
		name, _, err := parseVolumeSpec(v)
		if err != nil {
			continue
		}
		if name == "" {
			continue
		}
		key := "DOCKAN_VOLUME_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_", "/", "_").Replace(volumeEnvName(name)))
		host, _ := filepath.Abs(source)
		env[key] = host
	}
	return env
}

func VolumesDir() string {
	return filepath.Join(StoreRoot(), "volumes")
}

func CreateVolume(name string) error {
	if err := validateResourceName("nom de volume", name); err != nil {
		return err
	}
	path := filepath.Join(VolumesDir(), name)
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	fmt.Printf("%s\n", name)
	return nil
}

func RemoveVolume(name string) error {
	if err := validateResourceName("nom de volume", name); err != nil {
		return err
	}
	path := filepath.Join(VolumesDir(), name)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("volume introuvable: %s", name)
	}
	return os.RemoveAll(path)
}

func ListVolumes() error {
	entries, err := os.ReadDir(VolumesDir())
	if os.IsNotExist(err) {
		fmt.Printf("%-24s %s\n", "NAME", "PATH")
		return nil
	}
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	fmt.Printf("%-24s %s\n", "NAME", "PATH")
	for _, name := range names {
		fmt.Printf("%-24s %s\n", name, filepath.Join(VolumesDir(), name))
	}
	return nil
}

func InspectVolume(name string) error {
	if err := validateResourceName("nom de volume", name); err != nil {
		return err
	}
	path := filepath.Join(VolumesDir(), name)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("volume introuvable: %s", name)
	}
	fmt.Printf("name=%s\npath=%s\ncreated=%s\n", name, path, info.ModTime().Format("2006-01-02T15:04:05Z07:00"))
	return nil
}

func imageVolumeSpecs(meta map[string]string) []string {
	vols := strings.TrimSpace(meta["volumes"])
	if vols == "" {
		return nil
	}
	var specs []string
	for _, v := range strings.Split(vols, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			specs = append(specs, v)
		}
	}
	return specs
}

func parseVolumeSpec(spec string) (string, string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", "", fmt.Errorf("volume vide")
	}
	parts := strings.Split(spec, ":")
	if len(parts) == 1 {
		target := strings.TrimSpace(parts[0])
		if target == "" {
			return "", "", fmt.Errorf("volume invalide: %s", spec)
		}
		return volumeNameFromTarget(target), target, nil
	}
	if len(parts) < 2 || len(parts) > 3 {
		return "", "", fmt.Errorf("volume invalide: %s", spec)
	}
	source := strings.TrimSpace(parts[0])
	target := strings.TrimSpace(parts[1])
	mode := ""
	if len(parts) == 3 {
		mode = strings.TrimSpace(parts[2])
	}
	if source == "" || target == "" {
		return "", "", fmt.Errorf("volume invalide: %s", spec)
	}
	if mode != "" && mode != "ro" && mode != "rw" {
		return "", "", fmt.Errorf("mode de volume invalide: %s", mode)
	}
	return source, target, nil
}

func resolveVolumeSource(imagePath, spec string, runtime bool) (string, string, error) {
	source, target, err := parseVolumeSpec(spec)
	if err != nil {
		return "", "", err
	}
	if runtime && isHostVolumeSource(source) {
		hostPath := source
		if !filepath.IsAbs(hostPath) {
			abs, err := filepath.Abs(hostPath)
			if err != nil {
				return "", "", err
			}
			hostPath = abs
		}
		return hostPath, target, nil
	}
	if runtime {
		if err := validateResourceName("nom de volume", source); err != nil {
			return "", "", err
		}
		return filepath.Join(VolumesDir(), source), target, nil
	}
	return filepath.Join(imagePath, "volumes", source), target, nil
}

func isHostVolumeSource(source string) bool {
	return filepath.IsAbs(source) || strings.HasPrefix(source, ".") || strings.Contains(source, string(filepath.Separator))
}

func isRuntimeVolume(runtimeVolumes []string, spec string) bool {
	for _, runtime := range runtimeVolumes {
		if runtime == spec {
			return true
		}
	}
	return false
}

func volumeNameFromTarget(target string) string {
	name := strings.Trim(filepath.Clean(target), string(filepath.Separator))
	name = strings.NewReplacer("/", "_", ".", "_").Replace(name)
	if name == "" || name == "_" {
		return "volume"
	}
	return name
}

func volumeEnvName(name string) string {
	if isHostVolumeSource(name) {
		return volumeNameFromTarget(name)
	}
	return name
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func execCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
