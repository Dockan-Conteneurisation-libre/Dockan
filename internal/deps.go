package internal

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type DepsOptions struct {
	Manager  string
	Yes      bool
	DryRun   bool
	Packages []string
}

type packageManager struct {
	Name       string
	Binary     string
	InstallArg []string
	YesArg     string
	CheckArg   []string
}

var packageManagers = []packageManager{
	{Name: "apt", Binary: "apt-get", InstallArg: []string{"install"}, YesArg: "-y", CheckArg: []string{"--version"}},
	{Name: "dnf", Binary: "dnf", InstallArg: []string{"install"}, YesArg: "-y", CheckArg: []string{"--version"}},
	{Name: "apk", Binary: "apk", InstallArg: []string{"add"}, YesArg: "", CheckArg: []string{"--version"}},
	{Name: "pacman", Binary: "pacman", InstallArg: []string{"-S"}, YesArg: "--noconfirm", CheckArg: []string{"--version"}},
	{Name: "zypper", Binary: "zypper", InstallArg: []string{"install"}, YesArg: "-y", CheckArg: []string{"--version"}},
}

func CheckDepsManager(manager string) error {
	pm, err := resolvePackageManager(manager)
	if err != nil {
		return err
	}
	path, err := exec.LookPath(pm.Binary)
	if err != nil {
		return fmt.Errorf("gestionnaire introuvable: %s", pm.Name)
	}
	fmt.Printf("manager=%s\nbinary=%s\npath=%s\n", pm.Name, pm.Binary, path)
	return nil
}

func InstallDeps(opts DepsOptions) error {
	if len(opts.Packages) == 0 {
		return fmt.Errorf("aucun paquet demandé")
	}
	pm, err := resolvePackageManager(opts.Manager)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath(pm.Binary); err != nil {
		return fmt.Errorf("gestionnaire introuvable: %s", pm.Name)
	}
	args := append([]string{}, pm.InstallArg...)
	if opts.Yes && pm.YesArg != "" {
		args = append(args, pm.YesArg)
	}
	args = append(args, opts.Packages...)
	fmt.Printf("%s %s\n", pm.Binary, strings.Join(args, " "))
	if opts.DryRun {
		return nil
	}
	cmd := exec.Command(pm.Binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func resolvePackageManager(name string) (packageManager, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "auto" {
		for _, pm := range packageManagers {
			if commandExists(pm.Binary) {
				return pm, nil
			}
		}
		return packageManager{}, fmt.Errorf("aucun gestionnaire apt/dnf/apk/pacman/zypper trouvé")
	}
	for _, pm := range packageManagers {
		if pm.Name == name || pm.Binary == name {
			return pm, nil
		}
	}
	return packageManager{}, fmt.Errorf("gestionnaire inconnu: %s", name)
}
