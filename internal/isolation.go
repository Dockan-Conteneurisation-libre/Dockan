package internal

import (
	"fmt"
	"os"
	"os/exec"
)

// IsolationMethod représente une méthode d'isolation supportée
const (
	IsolationFirejail      = "firejail"
	IsolationSystemdNspawn = "systemd-nspawn"
	IsolationChroot        = "chroot"
)

// DetectIsolation tente de détecter la meilleure méthode d'isolation disponible
func DetectIsolation() string {
	if _, err := exec.LookPath("firejail"); err == nil {
		return IsolationFirejail
	}
	if _, err := exec.LookPath("systemd-nspawn"); err == nil {
		return IsolationSystemdNspawn
	}
	if _, err := exec.LookPath("chroot"); err == nil {
		return IsolationChroot
	}
	return ""
}

// RunWithIsolation exécute une commande dans le conteneur avec la méthode d'isolation choisie
func RunWithIsolation(isolation, rootfs, startScript string) error {
	var cmd *exec.Cmd
	switch isolation {
	case IsolationFirejail:
		cmd = exec.Command("firejail", "--quiet", "--private="+rootfs, "--shell="+startScript)
	case IsolationSystemdNspawn:
		cmd = exec.Command("systemd-nspawn", "-D", rootfs, "/start.sh")
	case IsolationChroot:
		cmd = exec.Command("sudo", "chroot", rootfs, "/start.sh")
	default:
		return fmt.Errorf("Aucune méthode d'isolation valide")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
