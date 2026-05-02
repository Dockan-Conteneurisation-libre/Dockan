package internal

import (
	"fmt"
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
		return []string{"nodejs"}
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
