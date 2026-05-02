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
