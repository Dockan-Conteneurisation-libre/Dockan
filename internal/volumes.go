package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MountVolumes monte les volumes déclarés dans meta.conf dans le rootfs du conteneur
func MountVolumes(imagePath string, meta map[string]string) error {
	vols, ok := meta["volumes"]
	if !ok || vols == "" {
		return nil
	}
	for _, v := range strings.Split(vols, ",") {
		parts := strings.SplitN(v, ":", 2)
		if len(parts) != 2 {
			continue
		}
		host := filepath.Join(imagePath, "volumes", parts[0])
		container := filepath.Join(imagePath, "rootfs", parts[1])
		os.MkdirAll(host, 0755)
		os.MkdirAll(container, 0755)
		// Montage par bind (nécessite privilèges)
		fmt.Printf("[dockan] Montage volume %s -> %s\n", host, container)
		if err := bindMount(host, container); err != nil {
			return err
		}
	}
	return nil
}

// bindMount effectue un montage bind (Linux uniquement)
func bindMount(source, target string) error {
	return execCommand("mount", "--bind", source, target)
}

func execCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
