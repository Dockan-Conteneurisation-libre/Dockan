package main

import (
	"dockan/internal"
	"fmt"
	"os"
	"strings"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Dockan - alternative libre à Docker\nUsage: dockan <commande> [options]\nEssayez 'dockan help'")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "__port-proxy":
		if len(os.Args) != 4 {
			fmt.Fprintln(os.Stderr, "Usage: dockan __port-proxy <listen> <target>")
			os.Exit(1)
		}
		if err := internal.RunPortProxy(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "help", "--help", "-h":
		printHelp()
	case "version", "--version", "-v":
		fmt.Println("dockan " + version)
	case "update", "upgrade":
		opts, err := parseUpdateOptions(os.Args[2:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
		if err := internal.UpdateCLI(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "run":
		imageRef, opts, err := parseRunCommand(os.Args[2:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
		imagePath, err := internal.ResolveImageReference(imageRef)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
		if opts.Detach {
			err = internal.StartDetachedContainer(imagePath, imageRef, opts)
		} else {
			err = internal.RunContainerLifecycle(imagePath, opts)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "ps":
		all := len(os.Args) > 2 && (os.Args[2] == "-a" || os.Args[2] == "--all")
		if err := internal.ListContainers(all); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "logs":
		if len(os.Args) != 3 {
			fmt.Println("Usage: dockan logs <conteneur>")
			os.Exit(1)
		}
		if err := internal.PrintContainerLogs(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "exec":
		if len(os.Args) < 4 {
			fmt.Println("Usage: dockan exec <conteneur> <commande> [args...]")
			os.Exit(1)
		}
		if err := internal.ExecContainer(os.Args[2], os.Args[3:]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "health":
		if len(os.Args) != 3 {
			fmt.Println("Usage: dockan health <conteneur>")
			os.Exit(1)
		}
		if err := internal.CheckContainerHealth(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "stop":
		if len(os.Args) != 3 {
			fmt.Println("Usage: dockan stop <conteneur>")
			os.Exit(1)
		}
		if err := internal.StopContainer(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "rm":
		if len(os.Args) != 3 {
			fmt.Println("Usage: dockan rm <conteneur>")
			os.Exit(1)
		}
		if err := internal.RemoveContainer(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "inspect":
		if len(os.Args) != 3 {
			fmt.Println("Usage: dockan inspect <conteneur>")
			os.Exit(1)
		}
		if err := internal.InspectContainer(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "build":
		opts, err := parseBuildOptions(os.Args[2:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
		if err := runBuild(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "images":
		if err := internal.PrintStoredImages(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "tag":
		if len(os.Args) != 4 {
			fmt.Println("Usage: dockan tag <source> <target:tag>")
			os.Exit(1)
		}
		if err := internal.TagImage(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "rmi":
		if len(os.Args) != 3 {
			fmt.Println("Usage: dockan rmi <image:tag>")
			os.Exit(1)
		}
		if err := internal.RemoveImage(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "push":
		if len(os.Args) < 3 || len(os.Args) > 4 {
			fmt.Println("Usage: dockan push <image:tag> [registry-dir]")
			os.Exit(1)
		}
		registryDir := ""
		if len(os.Args) == 4 {
			registryDir = os.Args[3]
		}
		if err := internal.PushImageToRegistry(os.Args[2], registryDir); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "pull":
		if len(os.Args) < 3 || len(os.Args) > 4 {
			fmt.Println("Usage: dockan pull <image:tag> [registry-dir]")
			os.Exit(1)
		}
		registryDir := ""
		if len(os.Args) == 4 {
			registryDir = os.Args[3]
		}
		if err := internal.PullImageFromRegistry(os.Args[2], registryDir); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "registry":
		if err := runRegistry(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "base":
		if err := runBase(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "deps":
		if err := runDeps(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "volume":
		if err := runVolume(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "network":
		if err := runNetwork(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "compose":
		if err := runCompose(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "service":
		if err := runService(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "new":
		if err := runNew(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "list":
		base := "."
		if len(os.Args) > 2 {
			base = os.Args[2]
		}
		internal.ListImages(base)
	case "init":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dockan init <image.dockan>")
			os.Exit(1)
		}
		if err := internal.InitImage(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "export":
		if len(os.Args) < 4 {
			fmt.Println("Usage: dockan export <image.dockan> <fichier.tar.gz>")
			os.Exit(1)
		}
		if err := internal.ExportImage(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "import":
		if len(os.Args) < 4 {
			fmt.Println("Usage: dockan import <fichier.tar.gz> <dossier.dockan>")
			os.Exit(1)
		}
		if err := internal.ImportImage(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "doctor":
		internal.PrintDoctor()
	default:
		fmt.Printf("Commande inconnue : %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runBase(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: dockan base <import|create|runtime> <tag> <rootfs-dir|rootfs.tar.gz>")
	}
	switch args[0] {
	case "import":
		if len(args) != 3 {
			return fmt.Errorf("Usage: dockan base import <tag> <rootfs-dir|rootfs.tar.gz>")
		}
		return internal.ImportBaseImage(args[1], args[2])
	case "create":
		if len(args) == 3 {
			return internal.ImportBaseImage(args[1], args[2])
		}
		if len(args) == 4 && args[2] == "--from" {
			return internal.ImportBaseImage(args[1], args[3])
		}
		return fmt.Errorf("Usage: dockan base create <tag> --from <rootfs-dir|rootfs.tar.gz>")
	case "runtime":
		if len(args) == 3 {
			return internal.CreateRuntimeBase(args[1], args[2])
		}
		if len(args) == 4 && args[2] == "--from" {
			return internal.CreateRuntimeBase(args[1], args[3])
		}
		return fmt.Errorf("Usage: dockan base runtime <tag> --from <rootfs-dir|rootfs.tar.gz>")
	default:
		return fmt.Errorf("commande base inconnue: %s", args[0])
	}
}

func runRegistry(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: dockan registry <ls> [registry-dir]")
	}
	registryDir := ""
	if len(args) > 1 {
		registryDir = args[1]
	}
	switch args[0] {
	case "ls", "list":
		return internal.PrintRegistryImages(registryDir)
	default:
		return fmt.Errorf("commande registry inconnue: %s", args[0])
	}
}

func parseRunOptions(args []string) (internal.RunOptions, error) {
	opts := internal.DefaultRunOptions()
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--isolation="):
			opts.Isolation = strings.TrimPrefix(arg, "--isolation=")
		case arg == "--no-isolation":
			opts.Isolation = internal.IsolationNone
		default:
			return opts, fmt.Errorf("option inconnue pour run: %s", arg)
		}
	}
	return opts, nil
}

func parseRunCommand(args []string) (string, internal.RunOptions, error) {
	opts := internal.DefaultRunOptions()
	var image string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-d", "--detach":
			opts.Detach = true
		case "--name":
			if i+1 >= len(args) {
				return image, opts, fmt.Errorf("--name attend une valeur")
			}
			opts.Name = args[i+1]
			i++
		case "-p", "--publish":
			if i+1 >= len(args) {
				return image, opts, fmt.Errorf("%s attend host:container", arg)
			}
			opts.Ports = append(opts.Ports, args[i+1])
			i++
		case "-e", "--env":
			if i+1 >= len(args) {
				return image, opts, fmt.Errorf("%s attend KEY=VALUE", arg)
			}
			opts.Env = append(opts.Env, args[i+1])
			i++
		case "-v", "--volume":
			if i+1 >= len(args) {
				return image, opts, fmt.Errorf("%s attend source:destination", arg)
			}
			opts.Volumes = append(opts.Volumes, args[i+1])
			i++
		case "--gui":
			opts.GUI = true
		case "--entrypoint":
			if i+1 >= len(args) {
				return image, opts, fmt.Errorf("--entrypoint attend une commande")
			}
			opts.Entrypoint = args[i+1]
			i++
		case "--restart":
			if i+1 >= len(args) {
				return image, opts, fmt.Errorf("--restart attend no|always|on-failure")
			}
			opts.Restart = args[i+1]
			i++
		case "--healthcheck":
			if i+1 >= len(args) {
				return image, opts, fmt.Errorf("--healthcheck attend une commande")
			}
			opts.Healthcheck = args[i+1]
			i++
		case "--memory", "-m":
			if i+1 >= len(args) {
				return image, opts, fmt.Errorf("%s attend une taille, ex: 512m", arg)
			}
			opts.Memory = args[i+1]
			i++
		case "--cpus":
			if i+1 >= len(args) {
				return image, opts, fmt.Errorf("--cpus attend un nombre, ex: 1.5")
			}
			opts.CPUs = args[i+1]
			i++
		case "--network":
			if i+1 >= len(args) {
				return image, opts, fmt.Errorf("--network attend un nom")
			}
			opts.Network = args[i+1]
			i++
		case "--alias":
			if i+1 >= len(args) {
				return image, opts, fmt.Errorf("--alias attend un nom DNS")
			}
			opts.Aliases = append(opts.Aliases, args[i+1])
			i++
		case "--no-isolation":
			opts.Isolation = internal.IsolationNone
		default:
			switch {
			case image != "":
				opts.Command = append(opts.Command, args[i:]...)
				i = len(args)
			case strings.HasPrefix(arg, "--isolation="):
				opts.Isolation = strings.TrimPrefix(arg, "--isolation=")
			case strings.HasPrefix(arg, "-"):
				return image, opts, fmt.Errorf("option inconnue pour run: %s", arg)
			case image == "":
				image = arg
			}
		}
	}
	if image == "" {
		return image, opts, fmt.Errorf("Usage: dockan run [options] <image|tag>")
	}
	if err := internal.ValidateRunOptions(opts); err != nil {
		return image, opts, err
	}
	return image, opts, nil
}

func parseBuildOptions(args []string) (internal.BuildOptions, error) {
	opts := internal.BuildOptions{Context: "."}
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-t", "--tag":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s attend un tag", arg)
			}
			opts.Tag = args[i+1]
			i++
		case "-f", "--file":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s attend un fichier", arg)
			}
			opts.File = args[i+1]
			i++
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) > 1 {
		return opts, fmt.Errorf("trop d'arguments pour build")
	}
	if len(positional) == 1 {
		opts.Context = positional[0]
	}
	if opts.Tag == "" {
		opts.Tag = strings.TrimSuffix(strings.TrimSuffix(opts.Context, "/"), ".dockan")
		if opts.Tag == "" || opts.Tag == "." {
			opts.Tag = "local"
		}
	}
	return opts, nil
}

func runBuild(opts internal.BuildOptions) error {
	if strings.HasSuffix(opts.Context, ".dockan") {
		if err := internal.BuildImage(opts.Context); err != nil {
			return err
		}
		if opts.Tag != "" {
			return internal.TagImage(opts.Context, opts.Tag)
		}
		return nil
	}
	return internal.BuildFromContext(opts)
}

func runDeps(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: dockan deps <check|install|runtime>")
	}
	switch args[0] {
	case "check":
		manager := "auto"
		if len(args) > 1 {
			manager = args[1]
		}
		return internal.CheckDepsManager(manager)
	case "install":
		opts := internal.DepsOptions{Manager: "auto"}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--manager":
				if i+1 >= len(args) {
					return fmt.Errorf("--manager attend apt|dnf|apk|pacman|zypper|auto")
				}
				opts.Manager = args[i+1]
				i++
			case "-y", "--yes":
				opts.Yes = true
			case "--dry-run":
				opts.DryRun = true
			default:
				if strings.HasPrefix(args[i], "-") {
					return fmt.Errorf("option inconnue: %s", args[i])
				}
				opts.Packages = append(opts.Packages, args[i])
			}
		}
		return internal.InstallDeps(opts)
	case "runtime":
		if len(args) < 2 {
			return fmt.Errorf("Usage: dockan deps runtime <image-ref> [--manager apt|dnf|apk|pacman|zypper|auto] [-y] [--dry-run]")
		}
		ref := args[1]
		opts := internal.DepsOptions{Manager: "auto"}
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--manager":
				if i+1 >= len(args) {
					return fmt.Errorf("--manager attend apt|dnf|apk|pacman|zypper|auto")
				}
				opts.Manager = args[i+1]
				i++
			case "-y", "--yes":
				opts.Yes = true
			case "--dry-run":
				opts.DryRun = true
			default:
				return fmt.Errorf("option inconnue: %s", args[i])
			}
		}
		return internal.InstallRuntimeDeps(ref, opts)
	default:
		return fmt.Errorf("commande deps inconnue: %s", args[0])
	}
}

func runVolume(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: dockan volume <ls|create|rm|inspect|backup|restore>")
	}
	switch args[0] {
	case "ls", "list":
		return internal.ListVolumes()
	case "create":
		if len(args) != 2 {
			return fmt.Errorf("Usage: dockan volume create <nom>")
		}
		return internal.CreateVolume(args[1])
	case "rm", "remove":
		if len(args) != 2 {
			return fmt.Errorf("Usage: dockan volume rm <nom>")
		}
		return internal.RemoveVolume(args[1])
	case "inspect":
		if len(args) != 2 {
			return fmt.Errorf("Usage: dockan volume inspect <nom>")
		}
		return internal.InspectVolume(args[1])
	case "backup":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("Usage: dockan volume backup <nom> [fichier.tar.gz]")
		}
		out := ""
		if len(args) == 3 {
			out = args[2]
		}
		return internal.BackupVolume(args[1], out)
	case "restore":
		if len(args) != 3 {
			return fmt.Errorf("Usage: dockan volume restore <nom> <fichier.tar.gz>")
		}
		return internal.RestoreVolume(args[1], args[2])
	default:
		return fmt.Errorf("commande volume inconnue: %s", args[0])
	}
}

func runNetwork(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: dockan network <ls|create|rm|enable|disable|hosts|refresh|doctor>")
	}
	switch args[0] {
	case "ls", "list":
		return internal.ListNetworks()
	case "doctor", "check":
		return internal.PrintNetworkDoctor()
	case "hosts":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		return internal.ListNetworkHosts(name)
	case "refresh":
		if len(args) != 2 {
			return fmt.Errorf("Usage: dockan network refresh <nom>")
		}
		return internal.RefreshNetwork(args[1])
	case "create":
		opts, err := parseNetworkCreateOptions(args[1:])
		if err != nil {
			return err
		}
		return internal.CreateNetworkWithOptions(opts)
	case "rm":
		if len(args) != 2 {
			return fmt.Errorf("Usage: dockan network rm <nom>")
		}
		return internal.RemoveNetwork(args[1])
	case "enable":
		if len(args) != 2 {
			return fmt.Errorf("Usage: dockan network enable <nom>")
		}
		return internal.EnableNetwork(args[1])
	case "disable":
		if len(args) != 2 {
			return fmt.Errorf("Usage: dockan network disable <nom>")
		}
		return internal.DisableNetwork(args[1])
	default:
		return fmt.Errorf("commande network inconnue: %s", args[0])
	}
}

func parseNetworkCreateOptions(args []string) (internal.NetworkOptions, error) {
	if len(args) < 1 {
		return internal.NetworkOptions{}, fmt.Errorf("Usage: dockan network create <nom> [--driver host-shared|bridge] [--subnet CIDR] [--gateway CIDR] [--bridge nom]")
	}
	opts := internal.NetworkOptions{Name: args[0], Driver: "host-shared"}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--driver":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--driver attend une valeur")
			}
			opts.Driver = args[i+1]
			i++
		case "--subnet":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--subnet attend un CIDR")
			}
			opts.Subnet = args[i+1]
			i++
		case "--gateway":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--gateway attend un CIDR")
			}
			opts.Gateway = args[i+1]
			i++
		case "--bridge":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--bridge attend un nom")
			}
			opts.Bridge = args[i+1]
			i++
		default:
			return opts, fmt.Errorf("option inconnue: %s", args[i])
		}
	}
	return opts, nil
}

func runCompose(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: dockan compose <up|down|redeploy|health> [-f dockan.yml]")
	}
	action := args[0]
	file, err := parseFileFlag(args[1:])
	if err != nil {
		return err
	}
	switch action {
	case "up":
		return internal.ComposeUp(file)
	case "down":
		return internal.ComposeDown(file)
	case "redeploy", "restart":
		return internal.ComposeRedeploy(file)
	case "health":
		return internal.ComposeHealth(file)
	default:
		return fmt.Errorf("commande compose inconnue: %s", action)
	}
}

func runService(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: dockan service <install|uninstall> [-f dockan.yml] [--name nom] [--user]")
	}
	action := args[0]
	opts, err := parseServiceOptions(args[1:])
	if err != nil {
		return err
	}
	switch action {
	case "install":
		return internal.InstallService(opts)
	case "uninstall", "remove", "rm":
		return internal.UninstallService(opts)
	default:
		return fmt.Errorf("commande service inconnue: %s", action)
	}
}

func runNew(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: dockan new <language> [dir] [--name nom] [--force]\nTemplates: %s", strings.Join(internal.AppTemplateNames(), ", "))
	}
	opts := internal.AppTemplateOptions{Language: args[0]}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 >= len(args) {
				return fmt.Errorf("--name attend un nom")
			}
			opts.Name = args[i+1]
			i++
		case "--force":
			opts.Force = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("option inconnue: %s", args[i])
			}
			if opts.Dir != "" {
				return fmt.Errorf("trop d'arguments pour new")
			}
			opts.Dir = args[i]
		}
	}
	return internal.CreateAppTemplate(opts)
}

func parseFileFlag(args []string) (string, error) {
	file := "dockan.yml"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-f", "--file":
			if i+1 >= len(args) {
				return file, fmt.Errorf("%s attend un fichier", args[i])
			}
			file = args[i+1]
			i++
		default:
			return file, fmt.Errorf("option inconnue: %s", args[i])
		}
	}
	return file, nil
}

func parseServiceOptions(args []string) (internal.ServiceOptions, error) {
	opts := internal.ServiceOptions{File: "dockan.yml"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-f", "--file":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s attend un fichier", args[i])
			}
			opts.File = args[i+1]
			i++
		case "--name":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--name attend un nom")
			}
			opts.Name = args[i+1]
			i++
		case "--user":
			opts.User = true
		default:
			return opts, fmt.Errorf("option inconnue: %s", args[i])
		}
	}
	return opts, nil
}

func parseUpdateOptions(args []string) (internal.UpdateOptions, error) {
	var opts internal.UpdateOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--version":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--version attend une version, ex: v0.1.1")
			}
			opts.Version = args[i+1]
			i++
		case "--system":
			opts.System = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return opts, fmt.Errorf("option inconnue: %s", args[i])
			}
			if opts.Version != "" {
				return opts, fmt.Errorf("version déjà définie: %s", opts.Version)
			}
			opts.Version = args[i]
		}
	}
	return opts, nil
}

func printHelp() {
	fmt.Print(`Dockan - alternative libre à Docker
Commandes :
  run [options] <image|tag>
                       Lance une image Dockan
  ps [-a]              Liste les conteneurs
  logs <conteneur>     Affiche les logs
  exec <conteneur> <commande>  Exécute une commande dans un conteneur
  health <conteneur>   Exécute le healthcheck du conteneur
  stop <conteneur>     Stoppe un conteneur détaché
  rm <conteneur>       Supprime un conteneur stoppé
  inspect <conteneur>  Affiche les métadonnées d'un conteneur
  build [-t nom:tag] [contexte]  Construit une image depuis un Dockanfile
  build [-t nom:tag] <image.dockan>  Construit/tag une image Dockan existante
  images               Liste les images locales
  tag <source> <target:tag>  Crée un tag local
  rmi <image:tag>      Supprime une image locale
  push <image:tag> [registry-dir]  Publie dans une registry locale dossier
  pull <image:tag> [registry-dir]  Récupère depuis une registry locale dossier
  registry ls [dir]    Liste une registry locale dossier
  base import <tag> <rootfs>  Importe une base locale
  base create <tag> --from <rootfs>  Crée une base locale
  base runtime <tag> --from <rootfs>  Crée une base runtime complète locale
  deps check|install|runtime
                       Vérifie ou installe des dépendances avec apt/dnf/apk...
  volume ls|create|rm|inspect|backup|restore
                       Gère les volumes locaux
  network ls|create|rm|enable|disable|hosts|refresh|doctor
                       Gère les réseaux Dockan
  compose up|down|redeploy|health
                       Lance, stoppe, redéploie ou vérifie un projet dockan.yml
  service install      Installe un projet dockan.yml comme service systemd
  service uninstall    Supprime le service systemd
  new <language> [dir] Crée une app Dockan: python,node,php,go,rust,java,ruby,binary
  list [dossier]       Liste les images Dockan
  init <image.dockan>  Crée un squelette d'image
  export <image.dockan> <fichier.tar.gz>  Exporte une image vers un fichier
  import <fichier.tar.gz> <dossier.dockan>  Importe une image depuis un fichier
  doctor               Vérifie les outils d'isolation disponibles
  version              Affiche la version
  update [--version vX.Y.Z] [--system]
                       Met à jour le binaire Dockan depuis GitHub Releases
  help                 Affiche cette aide

Options run :
  -d, --detach         Lance en arrière-plan
  --name <nom>         Nom du conteneur
  -p, --publish H:C    Déclare un port publié
  -e, --env K=V        Ajoute une variable d'environnement
  -v, --volume S:C     Monte un volume ou dossier local
  --gui                Monte les sockets GUI locaux si disponibles
  --entrypoint <cmd>   Remplace ENTRYPOINT
  --restart <mode>     no|always|on-failure
  --healthcheck <cmd>  Définit une commande de santé
  -m, --memory <taille> Limite mémoire avec prlimit, ex: 512m
  --cpus <nombre>      Déclare une limite CPU, ex: 1.5
  --network <nom>      Associe un réseau Dockan
  --alias <nom>        Ajoute un alias DNS/hosts sur le réseau Dockan
  --isolation=<mode>   auto|none|firejail|bubblewrap|systemd-nspawn|chroot

Options compose/service :
  -f, --file <fichier> Utilise un fichier dockan.yml
  --user               Installe le service systemd utilisateur

Options network create :
  --driver <driver>    host-shared|bridge
  --subnet <CIDR>      Sous-réseau bridge, ex: 10.89.0.0/24
  --gateway <CIDR>     Adresse du bridge, ex: 10.89.0.1/24
  --bridge <nom>       Nom de l'interface bridge Linux

Options deps install :
  --manager <nom>      auto|apt|dnf|apk|pacman|zypper
  -y, --yes            Accepte automatiquement si le gestionnaire le permet
  --dry-run            Affiche la commande sans installer

Options deps runtime :
  dockan deps runtime php:8.3 --dry-run
  sudo dockan deps runtime node:20 -y

Options new :
  --name <nom>         Nom de l'image/service généré
  --force              Remplace les fichiers générés existants
`)
}
