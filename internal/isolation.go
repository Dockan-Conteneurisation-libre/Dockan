package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	IsolationAuto          = "auto"
	IsolationNone          = "none"
	IsolationBubblewrap    = "bubblewrap"
	IsolationFirejail      = "firejail"
	IsolationSystemdNspawn = "systemd-nspawn"
	IsolationChroot        = "chroot"
)

func ResolveIsolation(requested string) (string, error) {
	if requested == "" || requested == IsolationAuto {
		if commandExists("firejail") {
			return IsolationFirejail, nil
		}
		if commandExists("bwrap") {
			return IsolationBubblewrap, nil
		}
		if os.Geteuid() == 0 && commandExists("systemd-nspawn") {
			return IsolationSystemdNspawn, nil
		}
		if os.Geteuid() == 0 && commandExists("chroot") {
			return IsolationChroot, nil
		}
		return IsolationNone, nil
	}

	switch requested {
	case IsolationNone:
		return IsolationNone, nil
	case IsolationFirejail:
		return requireCommand(requested, "firejail")
	case IsolationBubblewrap:
		return requireCommand(requested, "bwrap")
	case IsolationSystemdNspawn:
		if os.Geteuid() != 0 {
			return "", fmt.Errorf("systemd-nspawn nécessite root; utilisez sudo ou --isolation=none")
		}
		return requireCommand(requested, "systemd-nspawn")
	case IsolationChroot:
		if os.Geteuid() != 0 {
			return "", fmt.Errorf("chroot nécessite root; utilisez sudo ou --isolation=none")
		}
		return requireCommand(requested, "chroot")
	default:
		return "", fmt.Errorf("méthode d'isolation inconnue: %s", requested)
	}
}

func DetectIsolation() string {
	method, _ := ResolveIsolation(IsolationAuto)
	return method
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func requireCommand(method, command string) (string, error) {
	if !commandExists(command) {
		return "", fmt.Errorf("%s introuvable dans PATH", command)
	}
	return method, nil
}

func isolationCommand(method string, img *Image) (*exec.Cmd, error) {
	imagePath, err := filepath.Abs(img.Path)
	if err != nil {
		return nil, err
	}
	rootfs, err := filepath.Abs(img.RootfsDir)
	if err != nil {
		return nil, err
	}
	startScript, err := filepath.Abs(img.StartScript)
	if err != nil {
		return nil, err
	}

	if img.Meta["rootfs.mode"] == "oci" {
		return ociRootfsCommand(method, imagePath, rootfs, img.Meta["workdir"])
	}

	switch method {
	case IsolationNone:
		cmd := exec.Command("bash", startScript)
		cmd.Dir = imagePath
		return cmd, nil
	case IsolationFirejail:
		cmd := exec.Command("firejail", "--quiet", "--private="+rootfs, "--", "bash", startScript)
		cmd.Dir = imagePath
		return cmd, nil
	case IsolationBubblewrap:
		args := []string{
			"--proc", "/proc",
			"--dev", "/dev",
			"--tmpfs", "/tmp",
		}
		args = appendExistingBind(args, "--ro-bind", "/usr")
		args = appendExistingBind(args, "--ro-bind", "/bin")
		args = appendExistingBind(args, "--ro-bind", "/lib")
		args = appendExistingBind(args, "--ro-bind", "/lib64")
		args = append(args,
			"--bind", imagePath, "/dockan",
			"--chdir", "/dockan",
			"/bin/sh", "/dockan/start.sh",
		)
		return exec.Command("bwrap", args...), nil
	case IsolationSystemdNspawn:
		if _, err := os.Stat(filepath.Join(rootfs, "start.sh")); err != nil {
			return nil, fmt.Errorf("systemd-nspawn nécessite %s", filepath.Join(rootfs, "start.sh"))
		}
		return exec.Command("systemd-nspawn", "-D", rootfs, "/start.sh"), nil
	case IsolationChroot:
		if _, err := os.Stat(filepath.Join(rootfs, "start.sh")); err != nil {
			return nil, fmt.Errorf("chroot nécessite %s", filepath.Join(rootfs, "start.sh"))
		}
		return exec.Command("chroot", rootfs, "/start.sh"), nil
	default:
		return nil, fmt.Errorf("aucune méthode d'isolation valide")
	}
}

func ociRootfsCommand(method, imagePath, rootfs, workdir string) (*exec.Cmd, error) {
	if workdir == "" {
		workdir = "/"
	}
	switch method {
	case IsolationBubblewrap:
		args := []string{
			"--unshare-pid",
			"--bind", rootfs, "/",
			"--bind", imagePath, "/dockan",
			"--proc", "/proc",
			"--dev", "/dev",
			"--chdir", workdir,
			"/bin/sh", "/dockan/start.sh",
		}
		return exec.Command("bwrap", args...), nil
	case IsolationSystemdNspawn:
		return exec.Command("systemd-nspawn", "-D", rootfs, "/.dockan-start.sh"), nil
	case IsolationChroot:
		return exec.Command("chroot", rootfs, "/.dockan-start.sh"), nil
	case IsolationFirejail:
		if commandExists("bwrap") {
			return ociRootfsCommand(IsolationBubblewrap, imagePath, rootfs, workdir)
		}
		return nil, fmt.Errorf("image OCI Dockan nécessite bubblewrap, systemd-nspawn ou chroot")
	case IsolationNone:
		return nil, fmt.Errorf("image OCI Dockan nécessite une isolation rootfs: bubblewrap, systemd-nspawn ou chroot")
	default:
		return nil, fmt.Errorf("aucune méthode d'isolation OCI valide")
	}
}

func appendExistingBind(args []string, flag, path string) []string {
	if _, err := os.Stat(path); err == nil {
		args = append(args, flag, path, path)
	}
	return args
}

func RunWithIsolation(isolation, rootfs, startScript string) error {
	img := &Image{
		Path:        filepath.Dir(startScript),
		RootfsDir:   rootfs,
		StartScript: startScript,
	}
	cmd, err := isolationCommand(isolation, img)
	if err != nil {
		return err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	return cmd.Run()
}

func PrintDoctor() {
	fmt.Println("Dockan doctor")
	fmt.Printf("  firejail:        %s\n", availability("firejail"))
	fmt.Printf("  bubblewrap:      %s\n", availability("bwrap"))
	fmt.Printf("  systemd-nspawn:  %s\n", availability("systemd-nspawn"))
	fmt.Printf("  chroot:          %s\n", availability("chroot"))
	fmt.Printf("  nsenter:         %s\n", availability("nsenter"))
	fmt.Printf("  ip:              %s\n", availability("ip"))
	fmt.Printf("  iptables:        %s\n", availability("iptables"))
	fmt.Printf("  systemd-run:     %s\n", availability("systemd-run"))
	fmt.Printf("  prlimit:         %s\n", availability("prlimit"))
	fmt.Printf("  utilisateur:     uid=%d\n", os.Geteuid())

	method, err := ResolveIsolation(IsolationAuto)
	if err != nil {
		fmt.Printf("  isolation auto:  erreur: %v\n", err)
		return
	}
	fmt.Printf("  isolation auto:  %s\n", method)
	if method == IsolationNone {
		fmt.Println("  note:            aucun outil rootless détecté; Dockan exécutera start.sh sans isolation forte")
	}
}

func availability(command string) string {
	if commandExists(command) {
		return "ok"
	}
	return "absent"
}
