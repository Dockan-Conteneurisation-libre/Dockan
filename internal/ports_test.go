package internal

import "testing"

func TestPortBindAddressDefaultsToAllInterfaces(t *testing.T) {
	t.Setenv("DOCKAN_PORT_BIND_ADDR", "")

	if got := portBindAddress(); got != "0.0.0.0" {
		t.Fatalf("portBindAddress() = %q, want 0.0.0.0", got)
	}
}

func TestPortBindAddressCanBeRestrictedToLoopback(t *testing.T) {
	t.Setenv("DOCKAN_PORT_BIND_ADDR", "127.0.0.1")

	if got := portBindAddress(); got != "127.0.0.1" {
		t.Fatalf("portBindAddress() = %q, want 127.0.0.1", got)
	}
}
