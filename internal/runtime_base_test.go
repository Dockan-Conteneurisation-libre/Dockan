package internal

import (
	"reflect"
	"testing"
)

func TestRuntimeDepsPackagesMapsCommonRuntimes(t *testing.T) {
	tests := []struct {
		ref     string
		manager string
		want    []string
	}{
		{ref: "php:8.3", manager: "apt", want: []string{"php-cli"}},
		{ref: "frankenphp", manager: "apt", want: []string{"frankenphp"}},
		{ref: "frankenphp", manager: "dnf", want: []string{"frankenphp"}},
		{ref: "frankenphp", manager: "apk", want: []string{"frankenphp"}},
		{ref: "php:8.3", manager: "apk", want: []string{"php83"}},
		{ref: "node:20", manager: "dnf", want: []string{"nodejs", "npm"}},
		{ref: "python:3.12", manager: "pacman", want: []string{"python"}},
		{ref: "golang:1.22", manager: "apt", want: []string{"golang-go"}},
		{ref: "openjdk:21", manager: "dnf", want: []string{"java-21-openjdk-devel"}},
	}
	for _, tt := range tests {
		got, err := RuntimeDepsPackages(tt.ref, tt.manager)
		if err != nil {
			t.Fatalf("RuntimeDepsPackages(%q, %q) error = %v", tt.ref, tt.manager, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("RuntimeDepsPackages(%q, %q) = %#v, want %#v", tt.ref, tt.manager, got, tt.want)
		}
	}
}

func TestRuntimeDepsPackagesRejectsUnknownRuntime(t *testing.T) {
	if _, err := RuntimeDepsPackages("unknown:1", "apt"); err == nil {
		t.Fatal("RuntimeDepsPackages() expected error")
	}
}

func TestInstallFrankenPHPDepsDryRunSupportsLinuxManagers(t *testing.T) {
	for _, manager := range []string{"apt", "dnf", "apk", "pacman", "zypper"} {
		err := InstallFrankenPHPDeps(DepsOptions{Manager: manager, Yes: true, DryRun: true})
		if err != nil {
			t.Fatalf("InstallFrankenPHPDeps(%q) error = %v", manager, err)
		}
	}
}
