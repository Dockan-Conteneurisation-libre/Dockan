package internal

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const defaultInstallScriptURL = "https://raw.githubusercontent.com/Dockan-Conteneurisation-libre/Dockan/main/scripts/install.sh"

type UpdateOptions struct {
	Version string
	System  bool
}

func UpdateCLI(opts UpdateOptions) error {
	script, cleanup, err := downloadInstallScript(defaultInstallScriptURL)
	if err != nil {
		return err
	}
	defer cleanup()

	cmd := updateCommand(script, opts)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if opts.Version == "" {
		fmt.Println("Mise à jour Dockan depuis la dernière release...")
	} else {
		fmt.Printf("Mise à jour Dockan vers %s...\n", opts.Version)
	}
	return cmd.Run()
}

func downloadInstallScript(url string) (string, func(), error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", func() {}, fmt.Errorf("téléchargement du script impossible: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", func() {}, fmt.Errorf("téléchargement du script impossible: HTTP %d", resp.StatusCode)
	}

	dir, err := os.MkdirTemp("", "dockan-update-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		_ = os.RemoveAll(dir)
	}
	path := filepath.Join(dir, "install.sh")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}

func updateCommand(script string, opts UpdateOptions) *exec.Cmd {
	env := append(os.Environ(), "INSTALL_SOURCE=release")
	if opts.Version != "" {
		env = append(env, "VERSION="+opts.Version)
	}
	if opts.System && os.Geteuid() != 0 {
		args := []string{"env"}
		for _, item := range env {
			if hasEnvPrefix(item, "VERSION=") || hasEnvPrefix(item, "INSTALL_SOURCE=") {
				args = append(args, item)
			}
		}
		args = append(args, "sh", script)
		return exec.Command("sudo", args...)
	}
	cmd := exec.Command("sh", script)
	cmd.Env = env
	return cmd
}

func hasEnvPrefix(value, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}
