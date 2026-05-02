package internal

import (
	"path/filepath"
	"testing"
)

func TestValidateRunOptionsAcceptsKnownNetwork(t *testing.T) {
	t.Setenv("DOCKAN_HOME", filepath.Join(t.TempDir(), "store"))
	if err := CreateNetwork("appnet"); err != nil {
		t.Fatalf("CreateNetwork() error = %v", err)
	}

	err := ValidateRunOptions(RunOptions{
		Name:    "web-1",
		Env:     []string{"MODE=test", "EMPTY="},
		Ports:   []string{"8080:80"},
		Network: "appnet",
	})
	if err != nil {
		t.Fatalf("ValidateRunOptions() error = %v", err)
	}
}

func TestValidateRunOptionsRejectsBadEnvAndPorts(t *testing.T) {
	tests := []RunOptions{
		{Env: []string{"NO_EQUALS"}},
		{Env: []string{"1BAD=value"}},
		{Ports: []string{"8080"}},
		{Ports: []string{"abc:80"}},
		{Ports: []string{"8080:70000"}},
		{Isolation: "magic"},
	}

	for _, opts := range tests {
		if err := ValidateRunOptions(opts); err == nil {
			t.Fatalf("ValidateRunOptions(%+v) expected error", opts)
		}
	}
}

func TestValidateRunOptionsRejectsMissingNetwork(t *testing.T) {
	t.Setenv("DOCKAN_HOME", filepath.Join(t.TempDir(), "store"))

	err := ValidateRunOptions(RunOptions{Network: "missing"})
	if err == nil {
		t.Fatal("ValidateRunOptions() expected missing network error")
	}
}

func TestNetworkValidationRejectsUnsafeNames(t *testing.T) {
	t.Setenv("DOCKAN_HOME", filepath.Join(t.TempDir(), "store"))

	for _, name := range []string{"../bad", ".hidden", "bad/name", "host"} {
		if err := CreateNetwork(name); err == nil {
			t.Fatalf("CreateNetwork(%q) expected error", name)
		}
	}
}
