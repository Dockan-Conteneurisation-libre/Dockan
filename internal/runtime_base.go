package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type RuntimeBase struct {
	Name         string
	Command      string
	Alternatives []string
}

func ResolveHostRuntimeBase(ref string) (RuntimeBase, bool, error) {
	name := runtimeImageName(ref)
	runtime, ok := knownRuntimeBases()[name]
	if !ok {
		return RuntimeBase{}, false, nil
	}
	command := firstAvailableCommand(append([]string{runtime.Command}, runtime.Alternatives...))
	if command == "" {
		return runtime, true, fmt.Errorf("%s introuvable dans PATH; installez-le avec `sudo dockan deps runtime %s -y` ou importez une vraie base avec `dockan base import %s <rootfs>`", runtime.Command, ref, ref)
	}
	runtime.Command = command
	return runtime, true, nil
}

func RuntimeDepsPackages(ref, manager string) ([]string, error) {
	name := runtimeImageName(ref)
	if _, ok := knownRuntimeBases()[name]; !ok {
		return nil, fmt.Errorf("runtime inconnu pour %s", ref)
	}
	pm, err := resolvePackageManager(manager)
	if err != nil {
		return nil, err
	}
	packages := runtimePackagesByManager(ref, name, pm.Name)
	if len(packages) == 0 {
		return nil, fmt.Errorf("aucun paquet connu pour %s avec %s", ref, pm.Name)
	}
	return packages, nil
}

func InstallRuntimeDeps(ref string, opts DepsOptions) error {
	if runtimeImageName(ref) == "frankenphp" {
		return InstallFrankenPHPDeps(opts)
	}
	packages, err := RuntimeDepsPackages(ref, opts.Manager)
	if err != nil {
		return err
	}
	opts.Packages = append(opts.Packages, packages...)
	return InstallDeps(opts)
}

func runtimeImageName(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	ref = strings.TrimPrefix(ref, "docker.io/library/")
	ref = strings.TrimPrefix(ref, "library/")
	if slash := strings.LastIndex(ref, "/"); slash >= 0 {
		ref = ref[slash+1:]
	}
	if at := strings.Index(ref, "@"); at >= 0 {
		ref = ref[:at]
	}
	if colon := strings.Index(ref, ":"); colon >= 0 {
		ref = ref[:colon]
	}
	return strings.ToLower(ref)
}

func knownRuntimeBases() map[string]RuntimeBase {
	return map[string]RuntimeBase{
		"php":             {Name: "php", Command: "php"},
		"frankenphp":      {Name: "frankenphp", Command: "frankenphp"},
		"node":            {Name: "node", Command: "node"},
		"python":          {Name: "python", Command: "python3", Alternatives: []string{"python"}},
		"python3":         {Name: "python", Command: "python3", Alternatives: []string{"python"}},
		"golang":          {Name: "go", Command: "go"},
		"go":              {Name: "go", Command: "go"},
		"rust":            {Name: "rust", Command: "rustc"},
		"ruby":            {Name: "ruby", Command: "ruby"},
		"java":            {Name: "java", Command: "java"},
		"openjdk":         {Name: "java", Command: "java"},
		"eclipse-temurin": {Name: "java", Command: "java"},
		"amazoncorretto":  {Name: "java", Command: "java"},
		"perl":            {Name: "perl", Command: "perl"},
		"deno":            {Name: "deno", Command: "deno"},
		"bun":             {Name: "bun", Command: "bun"},
		"dotnet":          {Name: "dotnet", Command: "dotnet"},
		"mono":            {Name: "mono", Command: "mono"},
		"elixir":          {Name: "elixir", Command: "elixir"},
		"erlang":          {Name: "erlang", Command: "erl"},
		"lua":             {Name: "lua", Command: "lua"},
		"r-base":          {Name: "r", Command: "Rscript"},
		"r":               {Name: "r", Command: "Rscript"},
		"clojure":         {Name: "clojure", Command: "clojure"},
		"scala":           {Name: "scala", Command: "scala"},
		"swift":           {Name: "swift", Command: "swift"},
	}
}

func firstAvailableCommand(commands []string) string {
	for _, command := range commands {
		if commandExists(command) {
			return command
		}
	}
	return ""
}

func runtimePackagesByManager(ref, name, manager string) []string {
	switch name {
	case "frankenphp":
		switch manager {
		case "apt", "dnf", "apk":
			return []string{"frankenphp"}
		default:
			return nil
		}
	case "php":
		if manager == "apk" {
			majorMinor := runtimeMajorMinor(ref)
			if majorMinor != "" {
				return []string{"php" + majorMinor}
			}
		}
		if manager == "zypper" {
			return []string{"php8"}
		}
		return []string{"php-cli"}
	case "node":
		return []string{"nodejs", "npm"}
	case "python", "python3":
		if manager == "pacman" {
			return []string{"python"}
		}
		return []string{"python3"}
	case "golang", "go":
		switch manager {
		case "apt":
			return []string{"golang-go"}
		case "dnf":
			return []string{"golang"}
		default:
			return []string{"go"}
		}
	case "rust":
		if manager == "pacman" {
			return []string{"rust"}
		}
		return []string{"rust", "cargo"}
	case "ruby":
		return []string{"ruby"}
	case "java", "openjdk", "eclipse-temurin", "amazoncorretto":
		switch manager {
		case "apt":
			return []string{"openjdk-21-jdk-headless"}
		case "dnf", "zypper":
			return []string{"java-21-openjdk-devel"}
		case "apk":
			return []string{"openjdk21"}
		case "pacman":
			return []string{"jdk-openjdk"}
		default:
			return []string{"java"}
		}
	case "perl":
		return []string{"perl"}
	case "deno":
		return []string{"deno"}
	case "bun":
		return []string{"bun"}
	case "dotnet":
		return []string{"dotnet-sdk"}
	case "mono":
		return []string{"mono"}
	case "elixir":
		return []string{"elixir"}
	case "erlang":
		return []string{"erlang"}
	case "lua":
		return []string{"lua"}
	case "r-base", "r":
		if manager == "apt" || manager == "zypper" {
			return []string{"r-base"}
		}
		return []string{"R"}
	case "clojure":
		return []string{"clojure"}
	case "scala":
		return []string{"scala"}
	case "swift":
		return []string{"swift"}
	default:
		return nil
	}
}

func InstallFrankenPHPDeps(opts DepsOptions) error {
	pm, err := resolvePackageManager(opts.Manager)
	if err != nil {
		if opts.Manager == "" || opts.Manager == "auto" {
			return installFrankenPHPStandalone(opts)
		}
		return fmt.Errorf("%w; fallback disponible avec --manager auto", err)
	}
	switch pm.Name {
	case "dnf":
		return installFrankenPHPDNF(opts)
	case "apt":
		return installFrankenPHPAPT(opts)
	case "apk":
		return installFrankenPHPAPK(opts)
	case "pacman", "zypper":
		if err := installFrankenPHPStandaloneDeps(opts, pm); err != nil {
			return err
		}
		return installFrankenPHPStandalone(opts)
	default:
		return installFrankenPHPStandalone(opts)
	}
}

func installFrankenPHPDNF(opts DepsOptions) error {
	if err := runDepsCommand(opts, "dnf", appendYes([]string{"install"}, opts.Yes, "https://rpm.henderkes.com/static-php-1-0.noarch.rpm")...); err != nil {
		return err
	}
	if err := runDepsCommand(opts, "dnf", appendYes([]string{"module", "enable"}, opts.Yes, "php-zts:static-8.5")...); err != nil {
		return err
	}
	return runDepsCommand(opts, "dnf", appendYes([]string{"install"}, opts.Yes, "frankenphp")...)
}

func installFrankenPHPAPT(opts DepsOptions) error {
	version := frankenPHPStaticVersion()
	if err := runDepsCommand(opts, "apt-get", appendYes([]string{"install"}, opts.Yes, "ca-certificates", "curl")...); err != nil {
		return err
	}
	keyringDir := "/etc/apt/keyrings"
	keyring := filepath.Join(keyringDir, "static-php"+version+".asc")
	source := filepath.Join("/etc/apt/sources.list.d", "static-php"+version+".list")
	if opts.DryRun {
		fmt.Printf("mkdir -p %s\n", keyringDir)
	} else if err := os.MkdirAll(keyringDir, 0755); err != nil {
		return err
	}
	if err := runDepsCommand(opts, "curl", "https://pkg.henderkes.com/api/packages/"+version+"/debian/repository.key", "-o", keyring); err != nil {
		return err
	}
	sourceLine := "deb [signed-by=" + keyring + "] https://pkg.henderkes.com/api/packages/" + version + "/debian php-zts main\n"
	if err := writeDepsFile(source, sourceLine, opts.DryRun); err != nil {
		return err
	}
	if err := runDepsCommand(opts, "apt-get", "update"); err != nil {
		return err
	}
	return runDepsCommand(opts, "apt-get", appendYes([]string{"install"}, opts.Yes, "frankenphp")...)
}

func installFrankenPHPAPK(opts DepsOptions) error {
	version := frankenPHPStaticVersion()
	if err := runDepsCommand(opts, "apk", "add", "ca-certificates", "curl"); err != nil {
		return err
	}
	repository := "https://pkg.henderkes.com/api/packages/" + version + "/alpine/main/php-zts"
	if err := appendDepsLine("/etc/apk/repositories", repository, opts.DryRun); err != nil {
		return err
	}
	keyPath := filepath.Join("/etc/apk/keys", "static-php"+version+".rsa.pub")
	if err := runDepsCommand(opts, "curl", "-fsSL", "https://pkg.henderkes.com/api/packages/"+version+"/alpine/key", "-o", keyPath); err != nil {
		return err
	}
	if err := runDepsCommand(opts, "apk", "update"); err != nil {
		return err
	}
	return runDepsCommand(opts, "apk", "add", "frankenphp")
}

func installFrankenPHPStandaloneDeps(opts DepsOptions, pm packageManager) error {
	switch pm.Name {
	case "pacman":
		args := []string{"-S"}
		if opts.Yes {
			args = append(args, "--noconfirm")
		}
		args = append(args, "ca-certificates", "curl")
		return runDepsCommand(opts, pm.Binary, args...)
	case "zypper":
		return runDepsCommand(opts, pm.Binary, appendYes([]string{"install"}, opts.Yes, "ca-certificates", "curl")...)
	default:
		return nil
	}
}

func installFrankenPHPStandalone(opts DepsOptions) error {
	script := strings.Join([]string{
		`tmp="$(mktemp -d)"`,
		`trap 'rm -rf "$tmp"' EXIT`,
		`cd "$tmp"`,
		`curl -fsSL https://frankenphp.dev/install.sh -o install.sh`,
		`sh install.sh`,
		`install -m 0755 frankenphp /usr/local/bin/frankenphp`,
	}, " && ")
	return runDepsCommand(opts, "sh", "-c", script)
}

func frankenPHPStaticVersion() string {
	return "85"
}

func appendYes(prefix []string, yes bool, values ...string) []string {
	args := append([]string{}, prefix...)
	if yes {
		args = append(args, "-y")
	}
	return append(args, values...)
}

func runDepsCommand(opts DepsOptions, name string, args ...string) error {
	fmt.Printf("%s %s\n", name, strings.Join(args, " "))
	if opts.DryRun {
		return nil
	}
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func writeDepsFile(path, content string, dryRun bool) error {
	fmt.Printf("write %s\n", path)
	if dryRun {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func appendDepsLine(path, line string, dryRun bool) error {
	fmt.Printf("append %s: %s\n", path, line)
	if dryRun {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(data), line) {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(line + "\n")
	return err
}

func runtimeMajorMinor(ref string) string {
	_, version, ok := strings.Cut(ref, ":")
	if !ok {
		return ""
	}
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return ""
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return ""
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d%d", major, minor)
}
