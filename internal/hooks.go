package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// RunHook exécute un hook Dockan s'il existe (prestart, poststop, etc.)
func RunHook(imagePath, hookName string) error {
	hookPath := filepath.Join(imagePath, "hooks", hookName)
	if _, err := os.Stat(hookPath); err == nil {
		fmt.Printf("[dockan] Exécution du hook %s...\n", hookName)
		cmd := exec.Command("bash", hookPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		return cmd.Run()
	}
	return nil // Pas d'erreur si le hook n'existe pas
}

// RunContainerLifecycle exécute les hooks et le conteneur Dockan dans l'ordre (prestart, run, poststop)
func RunContainerLifecycle(imagePath string) error {
	meta, _ := ParseMeta(filepath.Join(imagePath, "meta.conf"))
	if err := MountVolumes(imagePath, meta); err != nil {
		return fmt.Errorf("Erreur montage volumes: %w", err)
	}
	if err := RunHook(imagePath, "prestart"); err != nil {
		return fmt.Errorf("Erreur hook prestart: %w", err)
	}
	if err := RunImage(imagePath); err != nil {
		return fmt.Errorf("Erreur exécution conteneur: %w", err)
	}
	if err := RunHook(imagePath, "poststop"); err != nil {
		return fmt.Errorf("Erreur hook poststop: %w", err)
	}
	return nil
}

// TODO: Ajoutez ici la logique de gestion des hooks Dockan (prestart, poststop, etc.)
