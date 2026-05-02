package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ComposeProject struct {
	Name     string
	Services []ComposeService
	Networks []string
}

type ComposeService struct {
	Name       string
	Image      string
	Build      string
	Ports      []string
	Env        []string
	Network    string
	Aliases    []string
	Isolation  string
	Volumes    []string
	Command    []string
	Entrypoint string
	DependsOn  []string
	Restart    string
	GUI        bool
	Memory     string
	CPUs       string
}

func ComposeUp(file string) error {
	project, err := LoadComposeFile(file)
	if err != nil {
		return err
	}
	for _, network := range project.Networks {
		if network == HostNetwork {
			continue
		}
		if err := ensureNetwork(network); err != nil {
			return err
		}
	}
	services, err := orderComposeServices(project.Services)
	if err != nil {
		return err
	}
	for _, service := range services {
		imageRef := service.Image
		if service.Build != "" {
			tag := imageRef
			if tag == "" {
				tag = project.Name + "-" + service.Name + ":latest"
			}
			if err := BuildFromContext(BuildOptions{Tag: tag, Context: composePath(file, service.Build)}); err != nil {
				return fmt.Errorf("build %s: %w", service.Name, err)
			}
			imageRef = tag
		}
		if imageRef == "" {
			return fmt.Errorf("service %s: image ou build requis", service.Name)
		}
		imagePath, err := ResolveImageReference(imageRef)
		if err != nil {
			return fmt.Errorf("service %s: %w", service.Name, err)
		}
		opts := RunOptions{
			Isolation:  service.Isolation,
			Detach:     true,
			Name:       project.Name + "-" + service.Name,
			Env:        service.Env,
			Ports:      service.Ports,
			Network:    service.Network,
			Aliases:    service.Aliases,
			Volumes:    service.Volumes,
			Command:    service.Command,
			Entrypoint: service.Entrypoint,
			Restart:    service.Restart,
			GUI:        service.GUI,
			Memory:     service.Memory,
			CPUs:       service.CPUs,
		}
		if opts.Isolation == "" {
			opts.Isolation = IsolationAuto
		}
		if err := StartDetachedContainer(imagePath, imageRef, opts); err != nil {
			if strings.Contains(err.Error(), "conteneur déjà existant") {
				fmt.Printf("%s existe déjà\n", opts.Name)
				continue
			}
			return fmt.Errorf("service %s: %w", service.Name, err)
		}
	}
	return nil
}

func orderComposeServices(services []ComposeService) ([]ComposeService, error) {
	byName := map[string]ComposeService{}
	for _, service := range services {
		byName[service.Name] = service
	}
	var ordered []ComposeService
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("depends_on circulaire: %s", name)
		}
		service, ok := byName[name]
		if !ok {
			return fmt.Errorf("service depends_on introuvable: %s", name)
		}
		visiting[name] = true
		for _, dep := range service.DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[name] = false
		visited[name] = true
		ordered = append(ordered, service)
		return nil
	}
	for _, service := range services {
		if err := visit(service.Name); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func ComposeDown(file string) error {
	project, err := LoadComposeFile(file)
	if err != nil {
		return err
	}
	for i := len(project.Services) - 1; i >= 0; i-- {
		name := project.Name + "-" + project.Services[i].Name
		if err := StopContainer(name); err != nil && !strings.Contains(err.Error(), "conteneur introuvable") {
			return err
		}
		if err := RemoveContainer(name); err != nil && !strings.Contains(err.Error(), "conteneur introuvable") {
			return err
		}
		fmt.Printf("%s supprimé\n", name)
	}
	return nil
}

func ComposeRedeploy(file string) error {
	if _, err := LoadComposeFile(file); err != nil {
		return err
	}
	if err := ComposeDown(file); err != nil {
		return err
	}
	return ComposeUp(file)
}

func LoadComposeFile(file string) (ComposeProject, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return ComposeProject{}, err
	}
	project := ComposeProject{Name: safeTag(strings.TrimSuffix(filepath.Base(filepath.Dir(absOrSame(file))), ".dockan"))}
	var section string
	var current *ComposeService
	var listKey string
	for lineNo, raw := range splitLines(string(data)) {
		line := stripInlineComment(raw)
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := countIndent(line)
		text := strings.TrimSpace(line)

		if indent == 0 {
			current = nil
			listKey = ""
			if strings.HasSuffix(text, ":") {
				section = strings.TrimSuffix(text, ":")
				continue
			}
			key, value, ok := strings.Cut(text, ":")
			if !ok {
				return project, fmt.Errorf("dockan.yml:%d: ligne invalide", lineNo+1)
			}
			switch strings.TrimSpace(key) {
			case "name":
				project.Name = safeTag(cleanScalar(value))
			default:
				return project, fmt.Errorf("dockan.yml:%d: clé inconnue: %s", lineNo+1, key)
			}
			continue
		}

		switch section {
		case "services":
			if indent == 2 && strings.HasSuffix(text, ":") {
				serviceName := strings.TrimSuffix(text, ":")
				if err := validateResourceName("nom de service", serviceName); err != nil {
					return project, err
				}
				project.Services = append(project.Services, ComposeService{Name: serviceName, Isolation: IsolationAuto})
				current = &project.Services[len(project.Services)-1]
				listKey = ""
				continue
			}
			if current == nil {
				return project, fmt.Errorf("dockan.yml:%d: service attendu", lineNo+1)
			}
			if indent == 4 {
				key, value, ok := strings.Cut(text, ":")
				if !ok {
					return project, fmt.Errorf("dockan.yml:%d: option de service invalide", lineNo+1)
				}
				key = strings.TrimSpace(key)
				value = strings.TrimSpace(value)
				listKey = ""
				switch key {
				case "image":
					current.Image = cleanScalar(value)
				case "build":
					current.Build = cleanScalar(value)
				case "network":
					current.Network = cleanScalar(value)
				case "isolation":
					current.Isolation = cleanScalar(value)
				case "command":
					current.Command = splitCommandValue(value)
				case "entrypoint":
					current.Entrypoint = cleanScalar(value)
				case "restart":
					current.Restart = cleanScalar(value)
				case "gui":
					current.GUI = cleanScalar(value) == "true"
				case "memory":
					current.Memory = cleanScalar(value)
				case "cpus":
					current.CPUs = cleanScalar(value)
				case "ports", "env", "volumes", "depends_on", "aliases":
					listKey = key
					if value != "" {
						items := splitCSV(value)
						for _, item := range items {
							addComposeListValue(current, key, item)
						}
					}
				default:
					return project, fmt.Errorf("dockan.yml:%d: option inconnue: %s", lineNo+1, key)
				}
				continue
			}
			if indent == 6 && strings.HasPrefix(text, "- ") && listKey != "" {
				addComposeListValue(current, listKey, strings.TrimSpace(strings.TrimPrefix(text, "- ")))
				continue
			}
			return project, fmt.Errorf("dockan.yml:%d: indentation invalide", lineNo+1)
		case "networks":
			if strings.HasPrefix(text, "- ") {
				project.Networks = append(project.Networks, cleanScalar(strings.TrimSpace(strings.TrimPrefix(text, "- "))))
				continue
			}
			if strings.HasSuffix(text, ":") {
				project.Networks = append(project.Networks, cleanScalar(strings.TrimSuffix(text, ":")))
				continue
			}
			return project, fmt.Errorf("dockan.yml:%d: réseau invalide", lineNo+1)
		default:
			return project, fmt.Errorf("dockan.yml:%d: section inconnue: %s", lineNo+1, section)
		}
	}
	if project.Name == "" {
		project.Name = "dockan"
	}
	if err := validateResourceName("nom de projet", project.Name); err != nil {
		return project, err
	}
	if len(project.Services) == 0 {
		return project, fmt.Errorf("aucun service dans dockan.yml")
	}
	normalizeComposeNetworks(&project)
	if err := validateComposeProject(project); err != nil {
		return project, err
	}
	return project, nil
}

func validateComposeProject(project ComposeProject) error {
	seen := map[string]bool{}
	for _, network := range project.Networks {
		if err := validateNetworkName(network); err != nil {
			return err
		}
		seen[network] = true
	}
	for _, service := range project.Services {
		if service.Image == "" && service.Build == "" {
			return fmt.Errorf("service %s: image ou build requis", service.Name)
		}
		opts := RunOptions{
			Name:       project.Name + "-" + service.Name,
			Env:        service.Env,
			Ports:      service.Ports,
			Network:    service.Network,
			Aliases:    service.Aliases,
			Isolation:  service.Isolation,
			Volumes:    service.Volumes,
			Command:    service.Command,
			Entrypoint: service.Entrypoint,
			Restart:    service.Restart,
			GUI:        service.GUI,
			Memory:     service.Memory,
			CPUs:       service.CPUs,
		}
		if service.Network != "" && service.Network != HostNetwork && !seen[service.Network] {
			return fmt.Errorf("service %s: réseau %s non déclaré", service.Name, service.Network)
		}
		for _, dep := range service.DependsOn {
			found := false
			for _, candidate := range project.Services {
				if candidate.Name == dep {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("service %s: depends_on introuvable: %s", service.Name, dep)
			}
		}
		if err := ValidateRunOptionsForCompose(opts, seen); err != nil {
			return fmt.Errorf("service %s: %w", service.Name, err)
		}
	}
	return nil
}

func ValidateRunOptionsForCompose(opts RunOptions, declaredNetworks map[string]bool) error {
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
	if err := validateRestart(opts.Restart); err != nil {
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
	if opts.Network != "" && opts.Network != HostNetwork && !declaredNetworks[opts.Network] {
		return fmt.Errorf("réseau non déclaré: %s", opts.Network)
	}
	return nil
}

func normalizeComposeNetworks(project *ComposeProject) {
	seen := map[string]bool{}
	for _, network := range project.Networks {
		if network != "" {
			seen[network] = true
		}
	}
	for i := range project.Services {
		if project.Services[i].Network == "" {
			project.Services[i].Network = HostNetwork
		}
		if project.Services[i].Network != HostNetwork {
			seen[project.Services[i].Network] = true
		}
	}
	project.Networks = project.Networks[:0]
	for network := range seen {
		project.Networks = append(project.Networks, network)
	}
	sort.Strings(project.Networks)
}

func ensureNetwork(name string) error {
	if err := validateNetworkName(name); err != nil {
		return err
	}
	if name == HostNetwork {
		return nil
	}
	if _, err := os.Stat(filepath.Join(NetworksDir(), name+".conf")); err == nil {
		return nil
	}
	return CreateNetwork(name)
}

func addComposeListValue(service *ComposeService, key, value string) {
	value = cleanScalar(value)
	if value == "" {
		return
	}
	switch key {
	case "ports":
		service.Ports = append(service.Ports, value)
	case "env":
		service.Env = append(service.Env, value)
	case "volumes":
		service.Volumes = append(service.Volumes, value)
	case "depends_on":
		service.DependsOn = append(service.DependsOn, value)
	case "aliases":
		service.Aliases = append(service.Aliases, value)
	}
}

func splitCommandValue(value string) []string {
	value = cleanScalar(value)
	if value == "" {
		return nil
	}
	return strings.Fields(value)
}

func composePath(file, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(filepath.Dir(absOrSame(file)), path)
}

func absOrSame(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func cleanScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'")
	return value
}

func splitCSV(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	parts := strings.Split(value, ",")
	var out []string
	for _, part := range parts {
		part = cleanScalar(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func stripInlineComment(line string) string {
	if idx := strings.Index(line, "#"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func countIndent(line string) int {
	count := 0
	for count < len(line) && line[count] == ' ' {
		count++
	}
	return count
}
