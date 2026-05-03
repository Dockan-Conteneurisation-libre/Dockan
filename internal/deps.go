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
	pm, err := resolvePackageManager(opts.Manager)
	if err != nil {
		return err
	}
	packages, err := expandDepsPackages(opts.Packages, pm.Name)
	if err != nil {
		return err
	}
	if len(packages) == 0 {
		return fmt.Errorf("aucun paquet demandé")
	}
	if _, err := exec.LookPath(pm.Binary); err != nil {
		return fmt.Errorf("gestionnaire introuvable: %s", pm.Name)
	}
	args := append([]string{}, pm.InstallArg...)
	if opts.Yes && pm.YesArg != "" {
		args = append(args, pm.YesArg)
	}
	args = append(args, packages...)
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

func expandDepsPackages(packages []string, manager string) ([]string, error) {
	var expanded []string
	seen := map[string]bool{}
	add := func(items ...string) {
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item == "" || seen[item] {
				continue
			}
			seen[item] = true
			expanded = append(expanded, item)
		}
	}
	for _, pkg := range packages {
		switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(pkg)), "@") {
		case "core", "dockan-core":
			add(coreDepsPackages(manager)...)
		case "tools", "utils", "common":
			add(toolsDepsPackages(manager)...)
		case "frontend", "front", "node-web", "nodeweb", "react", "vue", "vite":
			add(frontendDepsPackages(manager)...)
		case "network", "net":
			add(networkDepsPackages(manager)...)
		case "database", "db", "databases":
			add(databaseDepsPackages(manager)...)
		case "web", "http", "proxy":
			add(webDepsPackages(manager)...)
		case "build", "dev", "compile":
			add(buildDepsPackages(manager)...)
		case "debug", "diagnostic", "diagnostics":
			add(debugDepsPackages(manager)...)
		case "isolation", "sandbox":
			add(isolationDepsPackages(manager)...)
		case "full", "all", "recommended":
			add(coreDepsPackages(manager)...)
			add(toolsDepsPackages(manager)...)
			add(frontendDepsPackages(manager)...)
			add(networkDepsPackages(manager)...)
			add(databaseDepsPackages(manager)...)
			add(webDepsPackages(manager)...)
			add(buildDepsPackages(manager)...)
			add(debugDepsPackages(manager)...)
			add(isolationDepsPackages(manager)...)
		default:
			add(pkg)
		}
	}
	return expanded, nil
}

func coreDepsPackages(manager string) []string {
	switch manager {
	case "apt":
		return []string{"bash", "curl", "ca-certificates", "tar", "gzip", "xz-utils", "util-linux", "iproute2", "iptables", "procps"}
	case "dnf":
		return []string{"bash", "curl", "ca-certificates", "tar", "gzip", "xz", "util-linux", "iproute", "iptables", "procps-ng"}
	case "apk":
		return []string{"bash", "curl", "ca-certificates", "tar", "gzip", "xz", "util-linux", "iproute2", "iptables", "procps"}
	case "pacman":
		return []string{"bash", "curl", "ca-certificates", "tar", "gzip", "xz", "util-linux", "iproute2", "iptables", "procps-ng"}
	case "zypper":
		return []string{"bash", "curl", "ca-certificates", "tar", "gzip", "xz", "util-linux", "iproute2", "iptables", "procps"}
	default:
		return []string{"bash", "curl", "ca-certificates", "tar", "gzip", "util-linux"}
	}
}

func networkDepsPackages(manager string) []string {
	switch manager {
	case "apt":
		return []string{"iproute2", "iptables", "procps", "util-linux", "iputils-ping", "dnsutils", "netcat-openbsd", "socat"}
	case "dnf":
		return []string{"iproute", "iptables", "procps-ng", "util-linux", "iputils", "bind-utils", "nmap-ncat", "socat"}
	case "apk":
		return []string{"iproute2", "iptables", "procps", "util-linux", "iputils", "bind-tools", "netcat-openbsd", "socat"}
	case "pacman":
		return []string{"iproute2", "iptables", "procps-ng", "util-linux", "iputils", "bind", "openbsd-netcat", "socat"}
	case "zypper":
		return []string{"iproute2", "iptables", "procps", "util-linux", "iputils", "bind-utils", "netcat-openbsd", "socat"}
	default:
		return []string{"iproute2", "iptables", "procps", "util-linux", "iputils-ping", "socat"}
	}
}

func toolsDepsPackages(manager string) []string {
	switch manager {
	case "apt":
		return []string{"git", "jq", "rsync", "unzip", "zip", "openssl", "gnupg", "lsof", "file", "findutils"}
	case "dnf":
		return []string{"git", "jq", "rsync", "unzip", "zip", "openssl", "gnupg2", "lsof", "file", "findutils", "which"}
	case "apk":
		return []string{"git", "jq", "rsync", "unzip", "zip", "openssl", "gnupg", "lsof", "file", "findutils"}
	case "pacman":
		return []string{"git", "jq", "rsync", "unzip", "zip", "openssl", "gnupg", "lsof", "file", "findutils", "which"}
	case "zypper":
		return []string{"git", "jq", "rsync", "unzip", "zip", "openssl", "gpg2", "lsof", "file", "findutils", "which"}
	default:
		return []string{"git", "jq", "rsync", "unzip", "zip", "openssl", "lsof", "file"}
	}
}

func frontendDepsPackages(manager string) []string {
	switch manager {
	case "apt", "dnf", "apk", "pacman", "zypper":
		return []string{"nodejs", "npm"}
	default:
		return []string{"nodejs", "npm"}
	}
}

func databaseDepsPackages(manager string) []string {
	switch manager {
	case "apt":
		return []string{"sqlite3", "mariadb-client", "postgresql-client", "redis-tools"}
	case "dnf":
		return []string{"sqlite", "mariadb", "postgresql", "redis"}
	case "apk":
		return []string{"sqlite", "mariadb-client", "postgresql-client", "redis"}
	case "pacman":
		return []string{"sqlite", "mariadb-clients", "postgresql", "redis"}
	case "zypper":
		return []string{"sqlite3", "mariadb-client", "postgresql", "redis"}
	default:
		return []string{"sqlite3"}
	}
}

func webDepsPackages(manager string) []string {
	switch manager {
	case "apt", "dnf", "apk", "pacman", "zypper":
		return []string{"nginx", "caddy"}
	default:
		return []string{"nginx", "caddy"}
	}
}

func buildDepsPackages(manager string) []string {
	switch manager {
	case "apt":
		return []string{"make", "gcc", "g++", "pkg-config", "libc6-dev"}
	case "dnf":
		return []string{"make", "gcc", "gcc-c++", "pkgconf-pkg-config", "glibc-devel"}
	case "apk":
		return []string{"make", "gcc", "g++", "pkgconf", "musl-dev"}
	case "pacman":
		return []string{"base-devel", "pkgconf"}
	case "zypper":
		return []string{"make", "gcc", "gcc-c++", "pkg-config", "glibc-devel"}
	default:
		return []string{"make", "gcc", "pkg-config"}
	}
}

func debugDepsPackages(manager string) []string {
	switch manager {
	case "apt", "apk", "pacman", "zypper":
		return []string{"strace", "psmisc", "htop", "tcpdump"}
	case "dnf":
		return []string{"strace", "psmisc", "htop", "tcpdump"}
	default:
		return []string{"strace", "psmisc"}
	}
}

func isolationDepsPackages(manager string) []string {
	switch manager {
	case "apt", "dnf", "zypper":
		return []string{"firejail", "bubblewrap", "systemd-container"}
	case "apk":
		return []string{"bubblewrap"}
	case "pacman":
		return []string{"firejail", "bubblewrap", "systemd"}
	default:
		return []string{"firejail", "bubblewrap"}
	}
}
