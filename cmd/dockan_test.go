package main

import "testing"

func TestParseUpdateOptionsDefaults(t *testing.T) {
	opts, err := parseUpdateOptions(nil)
	if err != nil {
		t.Fatalf("parseUpdateOptions() error = %v", err)
	}
	if opts.Version != "" || opts.System {
		t.Fatalf("unexpected opts: %#v", opts)
	}
}

func TestParseUpdateOptionsVersionAndSystem(t *testing.T) {
	opts, err := parseUpdateOptions([]string{"--version", "v0.1.1", "--system"})
	if err != nil {
		t.Fatalf("parseUpdateOptions() error = %v", err)
	}
	if opts.Version != "v0.1.1" || !opts.System {
		t.Fatalf("unexpected opts: %#v", opts)
	}
}

func TestParseUpdateOptionsPositionalVersion(t *testing.T) {
	opts, err := parseUpdateOptions([]string{"v0.1.2"})
	if err != nil {
		t.Fatalf("parseUpdateOptions() error = %v", err)
	}
	if opts.Version != "v0.1.2" {
		t.Fatalf("unexpected opts: %#v", opts)
	}
}

func TestParsePSOptionsScopeAll(t *testing.T) {
	opts, err := parsePSOptions([]string{"-a", "--scope", "all"})
	if err != nil {
		t.Fatalf("parsePSOptions() error = %v", err)
	}
	if !opts.All || opts.Scope != "all" {
		t.Fatalf("unexpected opts: %#v", opts)
	}
}

func TestParsePSOptionsSystemShortcut(t *testing.T) {
	opts, err := parsePSOptions([]string{"--system"})
	if err != nil {
		t.Fatalf("parsePSOptions() error = %v", err)
	}
	if opts.All || opts.Scope != "system" {
		t.Fatalf("unexpected opts: %#v", opts)
	}
}

func TestParsePSOptionsRejectsUnknownScope(t *testing.T) {
	if _, err := parsePSOptions([]string{"--scope", "planet"}); err == nil {
		t.Fatal("parsePSOptions() expected error")
	}
}
