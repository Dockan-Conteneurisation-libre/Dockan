package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func RunHook(imagePath, hookName string) error {
	hookPath := filepath.Join(imagePath, "hooks", hookName)
	if _, err := os.Stat(hookPath); err == nil {
		fmt.Printf("[dockan] Exécution du hook %s...\n", hookName)
		cmd := exec.Command("bash", hookPath)
		cmd.Dir = imagePath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		return cmd.Run()
	}
	return nil
}

func RunContainerLifecycle(imagePath string, opts RunOptions) error {
	if err := ValidateRunOptions(opts); err != nil {
		return err
	}
	if IsBridgeNetwork(opts.Network) {
		return fmt.Errorf("réseau bridge nécessite le mode détaché: utilisez dockan run -d --network %s", opts.Network)
	}
	meta, err := ParseMeta(filepath.Join(imagePath, "meta.conf"))
	if err != nil {
		return err
	}
	cleanupVolumes, err := PrepareVolumesForRun(imagePath, meta, EffectiveRunVolumes(opts))
	if err != nil {
		return fmt.Errorf("erreur préparation volumes: %w", err)
	}
	defer cleanupVolumes()

	if err := RunHook(imagePath, "prestart"); err != nil {
		return fmt.Errorf("erreur hook prestart: %w", err)
	}
	if err := RunImage(imagePath, opts); err != nil {
		return fmt.Errorf("erreur exécution image: %w", err)
	}
	if err := RunHook(imagePath, "poststop"); err != nil {
		return fmt.Errorf("erreur hook poststop: %w", err)
	}
	return nil
}
