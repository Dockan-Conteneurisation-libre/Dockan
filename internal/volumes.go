package internal

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

type VolumeBind struct {
	Source       string
	Target       string
	ReadOnly     bool
	SourceIsFile bool
}

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

	binds, err := PrepareVolumeBindsForRun(imagePath, meta, runtimeVolumes)
	if err != nil {
		return cleanup, err
	}
	if len(binds) == 0 {
		return cleanup, nil
	}

	for _, bind := range binds {
		cleanContainer, _ := cleanVolumeTarget(bind.Target)
		container := filepath.Join(imagePath, "rootfs", cleanContainer)
		if os.Geteuid() == 0 {
			fmt.Printf("[dockan] Montage volume %s -> %s\n", bind.Source, container)
			if err := bindMount(bind.Source, container); err != nil {
				cleanup()
				return cleanup, err
			}
			mounted = append(mounted, container)
			continue
		}
		if err := ensureVolumeLink(bind.Source, container, bind.SourceIsFile); err != nil {
			cleanup()
			return cleanup, err
		}
		fmt.Printf("[dockan] Volume %s disponible via %s\n", bind.Source, container)
	}

	return cleanup, nil
}

func PrepareVolumeBindsForRun(imagePath string, meta map[string]string, runtimeVolumes []string) ([]VolumeBind, error) {
	specs := imageVolumeSpecs(meta)
	specs = append(specs, runtimeVolumes...)
	if len(specs) == 0 {
		return nil, nil
	}
	var binds []VolumeBind
	for _, v := range specs {
		bind, err := resolveVolumeBind(imagePath, v, isRuntimeVolume(runtimeVolumes, v))
		if err != nil {
			return nil, err
		}
		cleanContainer, err := cleanVolumeTarget(bind.Target)
		if err != nil {
			return nil, err
		}
		if err := prepareVolumeSource(bind); err != nil {
			return nil, err
		}
		if err := prepareVolumeTarget(filepath.Join(imagePath, "rootfs", cleanContainer), bind.SourceIsFile); err != nil {
			return nil, err
		}
		binds = append(binds, bind)
	}
	return binds, nil
}

func prepareVolumeSource(bind VolumeBind) error {
	if bind.SourceIsFile {
		return nil
	}
	return os.MkdirAll(bind.Source, 0755)
}

func prepareVolumeTarget(target string, fileTarget bool) error {
	if !fileTarget {
		if info, err := os.Lstat(target); err == nil && !info.IsDir() {
			return fmt.Errorf("volume cible déjà existante et non répertoire: %s", target)
		}
		return os.MkdirAll(target, 0755)
	}
	if info, err := os.Lstat(target); err == nil {
		if info.IsDir() {
			return fmt.Errorf("volume cible fichier déjà répertoire: %s", target)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	return file.Close()
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

func CleanupImageMounts(imagePath string) error {
	targets, err := mountedTargetsUnder(filepath.Join(imagePath, "rootfs"))
	if err != nil {
		return err
	}
	var firstErr error
	for i := len(targets) - 1; i >= 0; i-- {
		if err := syscall.Unmount(targets[i], 0); err != nil {
			if err := syscall.Unmount(targets[i], syscall.MNT_DETACH); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func mountedTargetsUnder(root string) ([]string, error) {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	return mountedTargetsUnderFromMountinfo(root, string(data)), nil
}

func mountedTargetsUnderFromMountinfo(root, mountinfo string) []string {
	root = filepath.Clean(root)
	prefix := root + string(filepath.Separator)
	var targets []string
	for _, line := range strings.Split(mountinfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		target := unescapeMountinfoPath(fields[4])
		cleanTarget := filepath.Clean(target)
		if cleanTarget == root || strings.HasPrefix(cleanTarget, prefix) {
			targets = append(targets, cleanTarget)
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		return len(targets[i]) < len(targets[j])
	})
	return targets
}

func unescapeMountinfoPath(path string) string {
	replacer := strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	)
	return replacer.Replace(path)
}

func ensureVolumeLink(source, target string, fileTarget bool) error {
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
		} else if fileTarget {
			if err := os.Remove(target); err != nil {
				return err
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

func BackupVolume(name, archivePath string) error {
	if err := validateResourceName("nom de volume", name); err != nil {
		return err
	}
	source := filepath.Join(VolumesDir(), name)
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
		return fmt.Errorf("volume introuvable: %s", name)
	}
	if archivePath == "" {
		archivePath = name + ".tar.gz"
	}
	file, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	if err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("type volume non supporté: %s", rel)
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(tw, src)
		return err
	}); err != nil {
		return err
	}
	fmt.Printf("Backup volume %s -> %s\n", name, archivePath)
	return nil
}

func RestoreVolume(name, archivePath string) error {
	if err := validateResourceName("nom de volume", name); err != nil {
		return err
	}
	if archivePath == "" {
		return fmt.Errorf("archive requise")
	}
	target := filepath.Join(VolumesDir(), name)
	if entries, err := os.ReadDir(target); err == nil && len(entries) > 0 {
		return fmt.Errorf("volume non vide: %s", name)
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		dest, err := cleanVolumeArchivePath(target, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("type archive non supporté: %s", header.Name)
		}
	}
	fmt.Printf("Restore volume %s <- %s\n", name, archivePath)
	return nil
}

func cleanVolumeArchivePath(root, name string) (string, error) {
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) || clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("chemin archive interdit: %s", name)
	}
	dest := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, dest)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("chemin archive interdit: %s", name)
	}
	return dest, nil
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

func resolveVolumeBind(imagePath, spec string, runtime bool) (VolumeBind, error) {
	source, target, err := resolveVolumeSource(imagePath, spec, runtime)
	if err != nil {
		return VolumeBind{}, err
	}
	mode := volumeSpecMode(spec)
	source, err = filepath.Abs(source)
	if err != nil {
		return VolumeBind{}, err
	}
	sourceIsFile := false
	if info, err := os.Stat(source); err == nil && !info.IsDir() {
		sourceIsFile = true
	}
	return VolumeBind{
		Source:       source,
		Target:       "/" + strings.TrimPrefix(filepath.Clean(target), string(filepath.Separator)),
		ReadOnly:     mode == "ro",
		SourceIsFile: sourceIsFile,
	}, nil
}

func volumeSpecMode(spec string) string {
	parts := strings.Split(strings.TrimSpace(spec), ":")
	if len(parts) == 3 {
		return strings.TrimSpace(parts[2])
	}
	return ""
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
