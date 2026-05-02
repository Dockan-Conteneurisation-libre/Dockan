package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const HostNetwork = "host"

func ValidateRunOptions(opts RunOptions) error {
	if err := validateIsolationMode(opts.Isolation); err != nil {
		return err
	}
	if opts.Name != "" {
		if err := validateContainerName(opts.Name); err != nil {
			return err
		}
	}
	for _, env := range opts.Env {
		if err := validateEnv(env); err != nil {
			return err
		}
	}
	for _, published := range opts.Ports {
		if err := validatePublishedPort(published); err != nil {
			return err
		}
	}
	for _, volume := range opts.Volumes {
		if err := validateRunVolume(volume); err != nil {
			return err
		}
	}
	if err := validateAliases(opts.Aliases); err != nil {
		return err
	}
	if opts.Network != "" {
		if err := validateNetworkExists(opts.Network); err != nil {
			return err
		}
	}
	if err := validateRestart(opts.Restart); err != nil {
		return err
	}
	if _, err := healthcheckCommand(opts.Healthcheck); err != nil {
		return err
	}
	if opts.Memory != "" {
		if _, err := parseMemoryBytes(opts.Memory); err != nil {
			return err
		}
	}
	if err := validateCPUs(opts.CPUs); err != nil {
		return err
	}
	return nil
}

func validateRunVolume(volume string) error {
	_, target, err := parseVolumeSpec(volume)
	if err != nil {
		return err
	}
	_, err = cleanVolumeTarget(target)
	return err
}

func validateRestart(value string) error {
	switch value {
	case "", "no", "always", "on-failure":
		return nil
	default:
		return fmt.Errorf("politique restart invalide: %s", value)
	}
}

func validateIsolationMode(mode string) error {
	switch mode {
	case "", IsolationAuto, IsolationNone, IsolationFirejail, IsolationBubblewrap, IsolationSystemdNspawn, IsolationChroot:
		return nil
	default:
		return fmt.Errorf("méthode d'isolation inconnue: %s", mode)
	}
}

func validateContainerName(name string) error {
	return validateResourceName("nom de conteneur", name)
}

func validateNetworkName(name string) error {
	return validateResourceName("nom de réseau", name)
}

func validateAliases(aliases []string) error {
	for _, alias := range aliases {
		if err := validateResourceName("alias réseau", alias); err != nil {
			return err
		}
	}
	return nil
}

func validateResourceName(label, name string) error {
	if name == "" {
		return fmt.Errorf("%s vide", label)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("%s invalide: %s", label, name)
	}
	first := name[0]
	if !isAlphaNum(first) {
		return fmt.Errorf("%s invalide: %s", label, name)
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if isAlphaNum(c) || c == '-' || c == '_' || c == '.' {
			continue
		}
		return fmt.Errorf("%s invalide: %s", label, name)
	}
	return nil
}

func validateEnv(env string) error {
	key, _, ok := strings.Cut(env, "=")
	if !ok || key == "" {
		return fmt.Errorf("variable d'environnement invalide: %s (format attendu KEY=VALUE)", env)
	}
	if !isEnvStart(key[0]) {
		return fmt.Errorf("nom de variable invalide: %s", key)
	}
	for i := 1; i < len(key); i++ {
		if !isEnvChar(key[i]) {
			return fmt.Errorf("nom de variable invalide: %s", key)
		}
	}
	return nil
}

func validatePublishedPort(published string) error {
	parts := strings.Split(published, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("port invalide: %s (format attendu host:container, exemple 8080:80)", published)
	}
	if err := validatePort(parts[0]); err != nil {
		return fmt.Errorf("port host invalide dans %s: %w", published, err)
	}
	if err := validatePort(parts[1]); err != nil {
		return fmt.Errorf("port container invalide dans %s: %w", published, err)
	}
	return nil
}

func validatePort(value string) error {
	port, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("nombre attendu")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("doit être entre 1 et 65535")
	}
	return nil
}

func validateNetworkExists(name string) error {
	if err := validateNetworkName(name); err != nil {
		return err
	}
	if name == HostNetwork {
		return nil
	}
	path := filepath.Join(NetworksDir(), name+".conf")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("réseau introuvable: %s (créez-le avec: dockan network create %s)", name, name)
	}
	return nil
}

func isAlphaNum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func isEnvStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isEnvChar(c byte) bool {
	return isEnvStart(c) || (c >= '0' && c <= '9')
}
