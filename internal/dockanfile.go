package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type BuildOptions struct {
	Tag     string
	Context string
	File    string
}

func BuildFromContext(opts BuildOptions) error {
	contextDir := opts.Context
	if contextDir == "" {
		contextDir = "."
	}
	absContext, err := filepath.Abs(contextDir)
	if err != nil {
		return err
	}
	dockanfile := opts.File
	if dockanfile == "" {
		dockanfile = filepath.Join(absContext, "Dockanfile")
		if _, err := os.Stat(dockanfile); err != nil {
			dockerfile := filepath.Join(absContext, "Dockerfile")
			if _, err := os.Stat(dockerfile); err == nil {
				dockanfile = dockerfile
			}
		}
	}
	if _, err := os.Stat(dockanfile); err != nil {
		return fmt.Errorf("Dockanfile ou Dockerfile introuvable dans %s", absContext)
	}

	tag := NormalizeTag(opts.Tag)
	imagePath := StoreImagePath(tag)
	if err := os.RemoveAll(imagePath); err != nil {
		return err
	}
	for _, subdir := range []string{"rootfs", "hooks", "volumes"} {
		if err := os.MkdirAll(filepath.Join(imagePath, subdir), 0755); err != nil {
			return err
		}
	}

	meta := map[string]string{
		"name": strings.Split(tag, ":")[0],
		"tag":  tag,
	}
	var cmdLine string
	var entrypointLine string
	var workdir string
	var envLines []string
	ignorePatterns := readDockerIgnore(absContext)
	stages := map[string]string{}
	var currentStage string
	stageIndex := 0
	seenFrom := false

	lines, err := readLines(dockanfile)
	if err != nil {
		return err
	}
	for lineNumber, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		instruction := strings.ToUpper(fields[0])
		rest := strings.TrimSpace(line[len(fields[0]):])

		switch instruction {
		case "FROM":
			if seenFrom {
				if currentStage != "" {
					stagePath := filepath.Join(imagePath, "stages", currentStage)
					if err := copyDir(filepath.Join(imagePath, "rootfs"), stagePath); err != nil {
						return err
					}
					stages[currentStage] = stagePath
				}
				stagePath := filepath.Join(imagePath, "stages", fmt.Sprintf("%d", stageIndex-1))
				if err := copyDir(filepath.Join(imagePath, "rootfs"), stagePath); err != nil {
					return err
				}
				stages[fmt.Sprintf("%d", stageIndex-1)] = stagePath
				if err := os.RemoveAll(filepath.Join(imagePath, "rootfs")); err != nil {
					return err
				}
				if err := os.MkdirAll(filepath.Join(imagePath, "rootfs"), 0755); err != nil {
					return err
				}
				cmdLine = ""
				entrypointLine = ""
				workdir = ""
				envLines = nil
			}
			baseRef, alias := parseFromInstruction(rest)
			currentStage = alias
			seenFrom = true
			stageIndex++
			if baseRef != "" && baseRef != "scratch" {
				if stageRootfs, ok := stages[baseRef]; ok {
					if err := copyDir(stageRootfs, filepath.Join(imagePath, "rootfs")); err != nil {
						return err
					}
					continue
				}
				base, err := ResolveImageReference(baseRef)
				if err != nil {
					runtime, ok, runtimeErr := ResolveHostRuntimeBase(baseRef)
					if runtimeErr != nil {
						return fmt.Errorf("Dockanfile:%d FROM %s: %w", lineNumber+1, baseRef, runtimeErr)
					}
					if !ok {
						return fmt.Errorf("Dockanfile:%d FROM: %w", lineNumber+1, err)
					}
					meta["base"] = baseRef
					meta["base.mode"] = "host-runtime"
					meta["runtime"] = runtime.Name
					meta["runtime.command"] = runtime.Command
					fmt.Printf("[dockan] FROM %s: using local host runtime %s, no Docker Hub download\n", baseRef, runtime.Command)
					continue
				}
				if err := copyDir(filepath.Join(base, "rootfs"), filepath.Join(imagePath, "rootfs")); err != nil {
					return err
				}
			}
		case "COPY", "ADD":
			parts := dockerInstructionArgs(rest)
			var fromSource string
			for len(parts) > 0 && strings.HasPrefix(parts[0], "--") {
				if value, ok := strings.CutPrefix(parts[0], "--from="); ok {
					fromSource = value
				}
				parts = parts[1:]
			}
			if len(parts) != 2 {
				return fmt.Errorf("Dockanfile:%d %s attend: %s source destination", lineNumber+1, instruction, instruction)
			}
			source := ""
			if fromSource != "" {
				stageRootfs, ok := stages[fromSource]
				if !ok {
					return fmt.Errorf("Dockanfile:%d stage introuvable: %s", lineNumber+1, fromSource)
				}
				source, err = cleanRootfsPath(stageRootfs, parts[0])
				if err != nil {
					return fmt.Errorf("Dockanfile:%d %w", lineNumber+1, err)
				}
			} else {
				source, err = cleanContextPath(absContext, parts[0])
				if err != nil {
					return fmt.Errorf("Dockanfile:%d %w", lineNumber+1, err)
				}
				if isIgnoredContextPath(absContext, source, ignorePatterns) && filepath.Clean(parts[0]) != "." {
					return fmt.Errorf("Dockanfile:%d source ignorée par .dockerignore: %s", lineNumber+1, parts[0])
				}
			}
			target, err := cleanRootfsPath(filepath.Join(imagePath, "rootfs"), parts[1])
			if err != nil {
				return fmt.Errorf("Dockanfile:%d %w", lineNumber+1, err)
			}
			info, err := os.Stat(source)
			if err != nil {
				return err
			}
			if info.IsDir() {
				if err := copyDirFiltered(absContext, source, target, ignorePatterns); err != nil {
					return err
				}
			} else {
				if err := copyFile(source, target, info.Mode()); err != nil {
					return err
				}
			}
		case "RUN":
			if rest == "" {
				return fmt.Errorf("Dockanfile:%d RUN vide", lineNumber+1)
			}
			runLine, err := normalizeDockerCommand(rest)
			if err != nil {
				return fmt.Errorf("Dockanfile:%d RUN: %w", lineNumber+1, err)
			}
			cmd := exec.Command("bash", "-lc", runLine)
			cmd.Dir = filepath.Join(imagePath, "rootfs", workdir)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Env = append(os.Environ(), "DOCKAN_ROOTFS="+filepath.Join(imagePath, "rootfs"))
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("Dockanfile:%d RUN: %w", lineNumber+1, err)
			}
		case "CMD":
			cmdLine, err = normalizeDockerCommand(rest)
			if err != nil {
				return fmt.Errorf("Dockanfile:%d CMD: %w", lineNumber+1, err)
			}
		case "ENTRYPOINT":
			entrypointLine, err = normalizeDockerCommand(rest)
			if err != nil {
				return fmt.Errorf("Dockanfile:%d ENTRYPOINT: %w", lineNumber+1, err)
			}
		case "ENV":
			key, value, ok := parseKeyValue(rest)
			if !ok || strings.TrimSpace(key) == "" {
				return fmt.Errorf("Dockanfile:%d ENV attend KEY=VALUE", lineNumber+1)
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if err := validateEnv(key + "=" + value); err != nil {
				return fmt.Errorf("Dockanfile:%d %w", lineNumber+1, err)
			}
			envLines = append(envLines, "export "+key+"="+shellQuote(value))
			meta["env"] = appendCSV(meta["env"], key+"="+value)
		case "WORKDIR":
			target, err := cleanRootfsPath(filepath.Join(imagePath, "rootfs"), rest)
			if err != nil {
				return fmt.Errorf("Dockanfile:%d %w", lineNumber+1, err)
			}
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			workdir = strings.TrimPrefix(filepath.Clean(rest), string(filepath.Separator))
			meta["workdir"] = "/" + workdir
		case "EXPOSE":
			for _, port := range strings.Fields(rest) {
				meta["ports"] = appendCSV(meta["ports"], port)
			}
		case "META":
			if i := strings.Index(rest, "="); i > 0 {
				meta[strings.TrimSpace(rest[:i])] = strings.TrimSpace(rest[i+1:])
			} else {
				return fmt.Errorf("Dockanfile:%d META attend key=value", lineNumber+1)
			}
		case "VOLUME":
			for _, volume := range parseDockerListOrFields(rest) {
				meta["volumes"] = appendCSV(meta["volumes"], volume)
			}
		case "LABEL":
			key, value, ok := parseKeyValue(rest)
			if !ok || strings.TrimSpace(key) == "" {
				return fmt.Errorf("Dockanfile:%d LABEL attend key=value", lineNumber+1)
			}
			meta["label."+strings.TrimSpace(key)] = strings.TrimSpace(value)
		case "ARG":
			key, value, ok := parseKeyValue(rest)
			if ok && key != "" {
				meta["arg."+strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		case "USER":
			meta["user"] = strings.TrimSpace(rest)
		case "SHELL":
			meta["shell"] = strings.TrimSpace(rest)
		case "STOPSIGNAL":
			meta["stopsignal"] = strings.TrimSpace(rest)
		case "HEALTHCHECK":
			meta["healthcheck"] = strings.TrimSpace(rest)
		default:
			return fmt.Errorf("Dockanfile:%d instruction inconnue: %s", lineNumber+1, instruction)
		}
	}

	if cmdLine == "" {
		cmdLine = "echo '(dockan) aucune commande CMD definie'"
	}
	if entrypointLine != "" {
		cmdLine = strings.TrimSpace(entrypointLine + " " + cmdLine)
	}
	startScript := "#!/usr/bin/env bash\nset -euo pipefail\ncd \"$(dirname \"$0\")/rootfs\"\n"
	if len(envLines) > 0 {
		startScript += strings.Join(envLines, "\n") + "\n"
	}
	if workdir != "" {
		startScript += "cd " + shellQuote(workdir) + "\n"
	}
	startScript += "if [ -n \"${DOCKAN_ENTRYPOINT:-}\" ]; then\n"
	startScript += "  if [ -n \"${DOCKAN_RUN_COMMAND:-}\" ]; then\n"
	startScript += "    eval \"$DOCKAN_ENTRYPOINT $DOCKAN_RUN_COMMAND\"\n"
	startScript += "  else\n"
	startScript += "    eval \"$DOCKAN_ENTRYPOINT " + strings.ReplaceAll(cmdLine, "\"", "\\\"") + "\"\n"
	startScript += "  fi\n"
	startScript += "elif [ -n \"${DOCKAN_RUN_COMMAND:-}\" ]; then\n"
	startScript += "  eval \"$DOCKAN_RUN_COMMAND\"\n"
	startScript += "else\n"
	startScript += cmdLine + "\n"
	startScript += "fi\n"
	if err := os.WriteFile(filepath.Join(imagePath, "start.sh"), []byte(startScript), 0755); err != nil {
		return err
	}
	if err := WriteMeta(filepath.Join(imagePath, "meta.conf"), meta); err != nil {
		return err
	}
	fmt.Printf("Successfully built %s\n", tag)
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func normalizeDockerCommand(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, "[") {
		return value, nil
	}
	var parts []string
	if err := json.Unmarshal([]byte(value), &parts); err != nil {
		return "", fmt.Errorf("forme exec Docker invalide: %w", err)
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("forme exec Docker vide")
	}
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, shellQuote(part))
	}
	return strings.Join(quoted, " "), nil
}

func parseFromInstruction(rest string) (string, string) {
	parts := strings.Fields(rest)
	if len(parts) == 0 {
		return "", ""
	}
	base := parts[0]
	for i := 1; i+1 < len(parts); i++ {
		if strings.EqualFold(parts[i], "AS") {
			return base, parts[i+1]
		}
	}
	return base, ""
}

func parseKeyValue(rest string) (string, string, bool) {
	key, value, ok := strings.Cut(rest, "=")
	if ok {
		return strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), "\"'"), true
	}
	parts := strings.Fields(rest)
	if len(parts) == 2 {
		return parts[0], strings.Trim(parts[1], "\"'"), true
	}
	return "", "", false
}

func dockerInstructionArgs(rest string) []string {
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "[") {
		return parseDockerListOrFields(rest)
	}
	return strings.Fields(rest)
}

func parseDockerListOrFields(rest string) []string {
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "[") {
		return strings.Fields(rest)
	}
	rest = strings.TrimPrefix(rest, "[")
	rest = strings.TrimSuffix(rest, "]")
	parts := strings.Split(rest, ",")
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "\"'")
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return splitLines(string(data)), nil
}

func readDockerIgnore(contextDir string) []string {
	lines, err := readLines(filepath.Join(contextDir, ".dockerignore"))
	if err != nil {
		return nil
	}
	var patterns []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		patterns = append(patterns, strings.TrimPrefix(line, "/"))
	}
	return patterns
}

func copyDirFiltered(contextDir, source, target string, ignorePatterns []string) error {
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != source && isIgnoredContextPath(contextDir, path, ignorePatterns) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(target, info.Mode())
		}
		outPath := filepath.Join(target, rel)
		if info.IsDir() {
			return os.MkdirAll(outPath, info.Mode())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, outPath, info.Mode())
	})
}

func isIgnoredContextPath(contextDir, path string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	rel, err := filepath.Rel(contextDir, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	base := filepath.Base(rel)
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if strings.HasSuffix(pattern, "/") {
			prefix := strings.TrimSuffix(pattern, "/")
			if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
				return true
			}
			continue
		}
		if ok, _ := filepath.Match(pattern, rel); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, base); ok {
			return true
		}
		if rel == pattern || strings.HasPrefix(rel, pattern+"/") {
			return true
		}
	}
	return false
}

func cleanContextPath(contextDir, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("chemin absolu interdit dans le contexte: %s", rel)
	}
	path := filepath.Clean(filepath.Join(contextDir, rel))
	if path != contextDir && !strings.HasPrefix(path, contextDir+string(filepath.Separator)) {
		return "", fmt.Errorf("chemin hors contexte interdit: %s", rel)
	}
	return path, nil
}

func cleanRootfsPath(rootfs, target string) (string, error) {
	clean := filepath.Clean(strings.TrimPrefix(target, string(filepath.Separator)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("chemin hors rootfs interdit: %s", target)
	}
	return filepath.Join(rootfs, clean), nil
}

func appendCSV(existing, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return existing
	}
	if strings.TrimSpace(existing) == "" {
		return value
	}
	return existing + "," + value
}
