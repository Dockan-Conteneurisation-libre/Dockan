package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadComposeFile(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "dockan.yml")
	data := `name: demo
services:
  web:
    build: .
    image: web:latest
    ports:
      - 8080:80
    env:
      - PORT=80
    volumes:
      - data:/data
    aliases:
      - api
      - backend
    depends_on:
      - db
    command: ./server --port 80
    entrypoint: /bin/sh -c
    restart: always
    healthcheck: CMD-SHELL curl -f http://127.0.0.1:80
    gui: true
    memory: 256m
    cpus: 1.5
    network: appnet
  db:
    image: db:latest
    network: appnet
networks:
  - appnet
`
	if err := os.WriteFile(file, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	project, err := LoadComposeFile(file)
	if err != nil {
		t.Fatalf("LoadComposeFile() error = %v", err)
	}
	if project.Name != "demo" {
		t.Fatalf("project.Name = %q", project.Name)
	}
	if len(project.Services) != 2 {
		t.Fatalf("services = %d", len(project.Services))
	}
	service := project.Services[0]
	if service.Name != "web" || service.Image != "web:latest" || service.Network != "appnet" {
		t.Fatalf("service = %+v", service)
	}
	if len(service.Ports) != 1 || service.Ports[0] != "8080:80" {
		t.Fatalf("ports = %#v", service.Ports)
	}
	if len(service.Env) != 1 || service.Env[0] != "PORT=80" {
		t.Fatalf("env = %#v", service.Env)
	}
	if len(service.Volumes) != 1 || service.Volumes[0] != "data:/data" {
		t.Fatalf("volumes = %#v", service.Volumes)
	}
	if len(service.Aliases) != 2 || service.Aliases[0] != "api" || service.Aliases[1] != "backend" {
		t.Fatalf("aliases = %#v", service.Aliases)
	}
	if len(service.DependsOn) != 1 || service.DependsOn[0] != "db" {
		t.Fatalf("depends_on = %#v", service.DependsOn)
	}
	if strings.Join(service.Command, " ") != "./server --port 80" || service.Entrypoint != "/bin/sh -c" || service.Restart != "always" || service.Healthcheck != "CMD-SHELL curl -f http://127.0.0.1:80" || !service.GUI || service.Memory != "256m" || service.CPUs != "1.5" {
		t.Fatalf("service extended fields = %+v", service)
	}
	ordered, err := orderComposeServices(project.Services)
	if err != nil {
		t.Fatalf("orderComposeServices() error = %v", err)
	}
	if ordered[0].Name != "db" || ordered[1].Name != "web" {
		t.Fatalf("ordered = %+v", ordered)
	}
}

func TestLoadComposeFileRejectsBadPort(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "dockan.yml")
	data := `name: demo
services:
  web:
    image: web:latest
    ports:
      - bad
`
	if err := os.WriteFile(file, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadComposeFile(file); err == nil {
		t.Fatal("LoadComposeFile() expected error")
	}
}

func TestServiceUnitContent(t *testing.T) {
	unit := serviceUnitContent(ServiceOptions{Name: "demo", User: true}, "/usr/bin/dockan", "/srv/demo/dockan.yml")
	for _, want := range []string{
		`ExecStart="/usr/bin/dockan" compose up -f "/srv/demo/dockan.yml"`,
		`ExecStop="/usr/bin/dockan" compose down -f "/srv/demo/dockan.yml"`,
		"WantedBy=default.target",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit missing %q:\n%s", want, unit)
		}
	}
}

func TestComposeRedeployRejectsMissingFile(t *testing.T) {
	if err := ComposeRedeploy(filepath.Join(t.TempDir(), "missing.yml")); err == nil {
		t.Fatal("ComposeRedeploy() expected error")
	}
}
