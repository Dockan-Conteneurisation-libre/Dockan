package internal

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

var ociHTTPClient = &http.Client{
	Timeout: 600 * time.Second,
}

type OCIRef struct {
	Registry   string
	Repository string
	Tag        string
	Normalized string
}

func parseOCIRef(raw string) OCIRef {
	raw = strings.TrimSpace(raw)
	tag := "latest"
	imagePart := raw
	if idx := strings.LastIndex(raw, ":"); idx != -1 {
		if slashIdx := strings.LastIndex(raw, "/"); slashIdx == -1 || idx > slashIdx {
			tag = raw[idx+1:]
			imagePart = raw[:idx]
		}
	}

	var registry, repository string
	parts := strings.Split(imagePart, "/")
	if len(parts) == 1 {
		registry = "registry-1.docker.io"
		repository = "library/" + parts[0]
	} else if len(parts) == 2 {
		if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" {
			registry = parts[0]
			repository = parts[1]
		} else {
			registry = "registry-1.docker.io"
			repository = parts[0] + "/" + parts[1]
		}
	} else {
		if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" {
			registry = parts[0]
			repository = strings.Join(parts[1:], "/")
		} else {
			registry = "registry-1.docker.io"
			repository = imagePart
		}
	}

	if registry == "docker.io" {
		registry = "registry-1.docker.io"
	}
	if registry == "registry-1.docker.io" && !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	norm := raw
	if strings.HasPrefix(norm, "docker.io/library/") {
		norm = strings.TrimPrefix(norm, "docker.io/library/")
	} else if strings.HasPrefix(norm, "index.docker.io/library/") {
		norm = strings.TrimPrefix(norm, "index.docker.io/library/")
	} else if strings.HasPrefix(norm, "docker.io/") {
		norm = strings.TrimPrefix(norm, "docker.io/")
	}
	if !strings.Contains(norm, ":") {
		norm = norm + ":" + tag
	}

	return OCIRef{
		Registry:   registry,
		Repository: repository,
		Tag:        tag,
		Normalized: norm,
	}
}

type OCIManifestList struct {
	Manifests []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Platform  struct {
			Architecture string `json:"architecture"`
			OS           string `json:"os"`
		} `json:"platform"`
	} `json:"manifests"`
}

type OCIManifest struct {
	Config struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	} `json:"layers"`
}

type OCIImageConfig struct {
	Config struct {
		User         string              `json:"User"`
		ExposedPorts map[string]struct{} `json:"ExposedPorts"`
		Env          []string            `json:"Env"`
		Entrypoint   []string            `json:"Entrypoint"`
		Cmd          []string            `json:"Cmd"`
		WorkingDir   string              `json:"WorkingDir"`
		StopSignal   string              `json:"StopSignal"`
	} `json:"config"`
}

// PullOCIImage pulls an image from an OCI registry using standard HTTP/JSON/TAR
// without any external dependencies.
func PullOCIImage(ref string, targetTag string) (*StoredImage, error) {
	parsed := parseOCIRef(ref)
	if targetTag == "" {
		targetTag = parsed.Normalized
	}
	targetTag = NormalizeTag(targetTag)

	imagePath := StoreImagePath(targetTag)
	tmpDir := imagePath + ".tmp"
	_ = os.RemoveAll(tmpDir)

	fmt.Printf("[dockan] Téléchargement OCI %s depuis https://%s/v2/%s (linux/%s)...\n",
		parsed.Tag, parsed.Registry, parsed.Repository, runtime.GOARCH)

	manifest, token, err := fetchOCIManifest(parsed)
	if err != nil {
		return nil, fmt.Errorf("échec de récupération du manifeste: %w", err)
	}

	config, token, err := fetchOCIConfig(parsed, manifest.Config.Digest, token)
	if err != nil {
		return nil, fmt.Errorf("échec de récupération de la configuration de l'image: %w", err)
	}

	for _, sub := range []string{"rootfs", "hooks", "volumes"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, sub), 0755); err != nil {
			_ = os.RemoveAll(tmpDir)
			return nil, err
		}
	}

	rootfsDir := filepath.Join(tmpDir, "rootfs")
	for i, layer := range manifest.Layers {
		shortDigest := layer.Digest
		if len(shortDigest) > 19 {
			shortDigest = shortDigest[:19]
		}
		fmt.Printf("[dockan] Couche %d/%d (%s, %s)...\n", i+1, len(manifest.Layers), shortDigest, formatByteSize(layer.Size))
		var layerErr error
		token, layerErr = downloadAndExtractLayer(parsed, layer.Digest, token, rootfsDir)
		if layerErr != nil {
			_ = os.RemoveAll(tmpDir)
			return nil, fmt.Errorf("échec d'extraction de la couche %d: %w", i+1, layerErr)
		}
	}

	name := strings.Split(targetTag, ":")[0]
	if lastSlash := strings.LastIndex(name, "/"); lastSlash >= 0 {
		name = name[lastSlash+1:]
	}

	meta := map[string]string{
		"name":        name,
		"tag":         targetTag,
		"source":      ref,
		"rootfs.mode": "oci",
	}

	if config.Config.WorkingDir != "" {
		meta["workdir"] = config.Config.WorkingDir
	} else {
		meta["workdir"] = "/"
	}

	if config.Config.User != "" {
		meta["user"] = config.Config.User
	}
	if config.Config.StopSignal != "" {
		meta["stopsignal"] = config.Config.StopSignal
	}

	var exposedPorts []string
	for p := range config.Config.ExposedPorts {
		portNum := strings.Split(p, "/")[0]
		if portNum != "" && !containsOCIString(exposedPorts, portNum) {
			exposedPorts = append(exposedPorts, portNum)
		}
	}
	sort.Strings(exposedPorts)
	if len(exposedPorts) > 0 {
		meta["ports"] = strings.Join(exposedPorts, ",")
	}

	startScript := generateOCIStartScriptStd(&config)
	if err := os.WriteFile(filepath.Join(tmpDir, "start.sh"), []byte(startScript), 0755); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}

	if err := WriteMeta(filepath.Join(tmpDir, "meta.conf"), meta); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}

	if err := os.RemoveAll(imagePath); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(imagePath), 0755); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}
	if err := os.Rename(tmpDir, imagePath); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}

	fmt.Printf("[dockan] Image OCI %s installée avec succès dans %s\n", targetTag, imagePath)
	return &StoredImage{Tag: targetTag, Path: imagePath, Name: name}, nil
}

func fetchOCIManifest(ref OCIRef) (*OCIManifest, string, error) {
	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", ref.Registry, ref.Repository, ref.Tag)
	accept := "application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.oci.image.index.v1+json"

	resp, token, err := ociGet(manifestURL, "", accept)
	if err != nil {
		return nil, token, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, token, fmt.Errorf("statut HTTP inattendu %d pour %s", resp.StatusCode, manifestURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, token, err
	}

	var list OCIManifestList
	if err := json.Unmarshal(body, &list); err == nil && len(list.Manifests) > 0 {
		var selectedDigest string
		for _, m := range list.Manifests {
			arch := m.Platform.Architecture
			if arch == "x86_64" {
				arch = "amd64"
			}
			osName := m.Platform.OS
			if (osName == "linux" || osName == "") && arch == runtime.GOARCH {
				selectedDigest = m.Digest
				break
			}
		}
		if selectedDigest == "" && len(list.Manifests) > 0 {
			selectedDigest = list.Manifests[0].Digest
		}

		targetURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", ref.Registry, ref.Repository, selectedDigest)
		subResp, newToken, err := ociGet(targetURL, token, accept)
		if err != nil {
			return nil, token, err
		}
		defer subResp.Body.Close()
		token = newToken
		body, err = io.ReadAll(subResp.Body)
		if err != nil {
			return nil, token, err
		}
	}

	var manifest OCIManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, token, fmt.Errorf("parsing du manifeste OCI échoué: %w", err)
	}

	if len(manifest.Layers) == 0 && manifest.Config.Digest == "" {
		return nil, token, fmt.Errorf("manifeste OCI vide ou non reconnu")
	}

	return &manifest, token, nil
}

func fetchOCIConfig(ref OCIRef, configDigest string, token string) (OCIImageConfig, string, error) {
	var cfg OCIImageConfig
	configURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", ref.Registry, ref.Repository, configDigest)
	resp, token, err := ociGet(configURL, token, "application/vnd.docker.container.image.v1+json, application/vnd.oci.image.config.v1+json")
	if err != nil {
		return cfg, token, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return cfg, token, fmt.Errorf("statut HTTP %d lors de la récupération de la config OCI", resp.StatusCode)
	}

	err = json.NewDecoder(resp.Body).Decode(&cfg)
	return cfg, token, err
}

func downloadAndExtractLayer(ref OCIRef, layerDigest string, token string, destDir string) (string, error) {
	layerURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", ref.Registry, ref.Repository, layerDigest)
	resp, token, err := ociGet(layerURL, token, "*/*")
	if err != nil {
		return token, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return token, fmt.Errorf("statut HTTP %d lors du téléchargement de la couche", resp.StatusCode)
	}

	var streamReader io.Reader = resp.Body
	gzReader, err := gzip.NewReader(resp.Body)
	if err == nil {
		defer gzReader.Close()
		streamReader = gzReader
	}

	return token, extractTarStream(streamReader, destDir)
}

func ociGet(reqURL, token, accept string) (*http.Response, string, error) {
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, token, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := ociHTTPClient.Do(req)
	if err != nil {
		return nil, token, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		authHdr := resp.Header.Get("Www-Authenticate")
		resp.Body.Close()

		realm, service, scope := parseBearerAuth(authHdr)
		if realm == "" {
			return nil, token, fmt.Errorf("requête 401 sans challenge Bearer valide")
		}
		newToken, err := fetchRegistryToken(realm, service, scope)
		if err != nil {
			return nil, token, fmt.Errorf("échec d'authentification Bearer: %w", err)
		}
		token = newToken

		retryReq, err := http.NewRequest("GET", reqURL, nil)
		if err != nil {
			return nil, token, err
		}
		if accept != "" {
			retryReq.Header.Set("Accept", accept)
		}
		retryReq.Header.Set("Authorization", "Bearer "+token)
		resp, err = ociHTTPClient.Do(retryReq)
		if err != nil {
			return nil, token, err
		}
	}

	return resp, token, nil
}

func parseBearerAuth(header string) (realm, service, scope string) {
	if !strings.HasPrefix(header, "Bearer ") {
		return "", "", ""
	}
	raw := strings.TrimPrefix(header, "Bearer ")
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		val = strings.Trim(val, "\"")
		switch key {
		case "realm":
			realm = val
		case "service":
			service = val
		case "scope":
			scope = val
		}
	}
	return
}

func fetchRegistryToken(realm, service, scope string) (string, error) {
	u, err := url.Parse(realm)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if service != "" {
		q.Set("service", service)
	}
	if scope != "" {
		q.Set("scope", scope)
	}
	u.RawQuery = q.Encode()

	resp, err := ociHTTPClient.Get(u.String())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth endpoint error: status %d", resp.StatusCode)
	}

	var data struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	if data.Token != "" {
		return data.Token, nil
	}
	return data.AccessToken, nil
}

func generateOCIStartScriptStd(cfg *OCIImageConfig) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("set -eu\n")

	for _, env := range cfg.Config.Env {
		key, val, ok := strings.Cut(env, "=")
		if ok && key != "" {
			b.WriteString(fmt.Sprintf("export %s=%s\n", key, shellQuote(val)))
		}
	}

	b.WriteString("if [ -n \"${DOCKAN_ENTRYPOINT:-}\" ]; then\n")
	b.WriteString("  if [ -n \"${DOCKAN_RUN_COMMAND:-}\" ]; then\n")
	b.WriteString("    exec sh -lc \"$DOCKAN_ENTRYPOINT $DOCKAN_RUN_COMMAND\"\n")
	b.WriteString("  fi\n")
	b.WriteString("  exec sh -lc \"$DOCKAN_ENTRYPOINT\"\n")
	b.WriteString("fi\n")

	entrypoint := cfg.Config.Entrypoint
	cmd := cfg.Config.Cmd

	if len(entrypoint) > 0 {
		var epQuoted []string
		for _, arg := range entrypoint {
			epQuoted = append(epQuoted, shellQuote(arg))
		}
		epStr := strings.Join(epQuoted, " ")

		b.WriteString("if [ -n \"${DOCKAN_RUN_COMMAND:-}\" ]; then\n")
		b.WriteString(fmt.Sprintf("  exec %s $DOCKAN_RUN_COMMAND\n", epStr))
		b.WriteString("fi\n")
	} else {
		b.WriteString("if [ -n \"${DOCKAN_RUN_COMMAND:-}\" ]; then\n")
		b.WriteString("  exec sh -lc \"$DOCKAN_RUN_COMMAND\"\n")
		b.WriteString("fi\n")
	}

	workdir := strings.TrimSpace(cfg.Config.WorkingDir)
	if workdir != "" && workdir != "/" {
		b.WriteString(fmt.Sprintf("mkdir -p %s\n", shellQuote(workdir)))
		b.WriteString(fmt.Sprintf("cd %s\n", shellQuote(workdir)))
	}

	var fullCmd []string
	fullCmd = append(fullCmd, entrypoint...)
	fullCmd = append(fullCmd, cmd...)

	if len(fullCmd) == 0 {
		b.WriteString("exec /bin/sh\n")
	} else {
		b.WriteString("exec")
		for _, arg := range fullCmd {
			b.WriteString(" " + shellQuote(arg))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func extractTarStream(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			continue
		}

		baseName := filepath.Base(cleanName)
		target := filepath.Join(dest, cleanName)
		parentDir := filepath.Dir(target)

		// Handle OCI Whiteouts
		if baseName == ".wh..wh..opq" {
			// Opaque whiteout: clear the contents of parentDir
			if entries, err := os.ReadDir(parentDir); err == nil {
				for _, entry := range entries {
					_ = os.RemoveAll(filepath.Join(parentDir, entry.Name()))
				}
			}
			continue
		} else if strings.HasPrefix(baseName, ".wh.") {
			deletedName := strings.TrimPrefix(baseName, ".wh.")
			_ = os.RemoveAll(filepath.Join(parentDir, deletedName))
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if fi, err := os.Lstat(target); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}

		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(parentDir, 0755); err != nil {
				return err
			}
			_ = os.Remove(target)
			mode := hdr.FileInfo().Mode().Perm()
			if mode == 0 {
				mode = 0644
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
			_ = os.Chmod(target, mode)

		case tar.TypeSymlink:
			if err := os.MkdirAll(parentDir, 0755); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}

		case tar.TypeLink:
			if err := os.MkdirAll(parentDir, 0755); err != nil {
				return err
			}
			_ = os.Remove(target)
			oldPath := filepath.Join(dest, filepath.Clean(hdr.Linkname))
			if err := os.Link(oldPath, target); err != nil {
				_ = copyFile(oldPath, target, 0644)
			}

		default:
			// Ignore device nodes, fifos, etc. in rootless/container extractions
		}
	}
	return nil
}

func formatByteSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func containsOCIString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
