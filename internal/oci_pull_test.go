package internal

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOCIRef(t *testing.T) {
	tests := []struct {
		input    string
		registry string
		repo     string
		tag      string
		norm     string
	}{
		{
			input:    "alpine",
			registry: "registry-1.docker.io",
			repo:     "library/alpine",
			tag:      "latest",
			norm:     "alpine:latest",
		},
		{
			input:    "alpine:3.19",
			registry: "registry-1.docker.io",
			repo:     "library/alpine",
			tag:      "3.19",
			norm:     "alpine:3.19",
		},
		{
			input:    "nginx:alpine",
			registry: "registry-1.docker.io",
			repo:     "library/nginx",
			tag:      "alpine",
			norm:     "nginx:alpine",
		},
		{
			input:    "prestashop/prestashop:latest",
			registry: "registry-1.docker.io",
			repo:     "prestashop/prestashop",
			tag:      "latest",
			norm:     "prestashop/prestashop:latest",
		},
		{
			input:    "ghcr.io/immich-app/immich-server:release",
			registry: "ghcr.io",
			repo:     "immich-app/immich-server",
			tag:      "release",
			norm:     "ghcr.io/immich-app/immich-server:release",
		},
		{
			input:    "docker.io/library/redis:alpine",
			registry: "registry-1.docker.io",
			repo:     "library/redis",
			tag:      "alpine",
			norm:     "redis:alpine",
		},
	}

	for _, tc := range tests {
		res := parseOCIRef(tc.input)
		if res.Registry != tc.registry {
			t.Errorf("parseOCIRef(%q).Registry = %q, want %q", tc.input, res.Registry, tc.registry)
		}
		if res.Repository != tc.repo {
			t.Errorf("parseOCIRef(%q).Repository = %q, want %q", tc.input, res.Repository, tc.repo)
		}
		if res.Tag != tc.tag {
			t.Errorf("parseOCIRef(%q).Tag = %q, want %q", tc.input, res.Tag, tc.tag)
		}
		if res.Normalized != tc.norm {
			t.Errorf("parseOCIRef(%q).Normalized = %q, want %q", tc.input, res.Normalized, tc.norm)
		}
	}
}

func TestGenerateOCIStartScriptStd(t *testing.T) {
	cfg := &OCIImageConfig{}
	cfg.Config.Env = []string{"PATH=/usr/bin:/bin", "NODE_ENV=production"}
	cfg.Config.Entrypoint = []string{"/docker-entrypoint.sh"}
	cfg.Config.Cmd = []string{"node", "server.js"}
	cfg.Config.WorkingDir = "/app"

	script := generateOCIStartScriptStd(cfg)

	if !strings.Contains(script, "export PATH='/usr/bin:/bin'") {
		t.Errorf("expected PATH export in script, got: %s", script)
	}
	if !strings.Contains(script, "export NODE_ENV='production'") {
		t.Errorf("expected NODE_ENV export in script, got: %s", script)
	}
	if !strings.Contains(script, "mkdir -p '/app'") {
		t.Errorf("expected workdir mkdir in script, got: %s", script)
	}
	if !strings.Contains(script, "exec '/docker-entrypoint.sh' 'node' 'server.js'") {
		t.Errorf("expected full exec command in script, got: %s", script)
	}
}

func TestEnsureImageAvailableLocalRejection(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))

	// Non-existent :local image should return error without attempting network
	_, err := EnsureImageAvailable("nonexistent:local")
	if err == nil {
		t.Fatal("expected error for nonexistent :local image")
	}
	if !strings.Contains(err.Error(), "image introuvable") {
		t.Fatalf("unexpected error: %v", err)
	}
}
