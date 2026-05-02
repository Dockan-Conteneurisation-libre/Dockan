package internal

import "testing"

func TestParseMemoryBytes(t *testing.T) {
	tests := map[string]int64{
		"512m": 512 * 1024 * 1024,
		"1g":   1024 * 1024 * 1024,
		"64kb": 64 * 1024,
	}
	for input, want := range tests {
		got, err := parseMemoryBytes(input)
		if err != nil {
			t.Fatalf("parseMemoryBytes(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("parseMemoryBytes(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestValidateCPUs(t *testing.T) {
	if err := validateCPUs("1.5"); err != nil {
		t.Fatalf("validateCPUs() error = %v", err)
	}
	if err := validateCPUs("0"); err == nil {
		t.Fatal("validateCPUs() expected error")
	}
}

func TestCPUQuotaPercent(t *testing.T) {
	got, err := cpuQuotaPercent("1.5")
	if err != nil {
		t.Fatalf("cpuQuotaPercent() error = %v", err)
	}
	if got != "150%" {
		t.Fatalf("cpuQuotaPercent() = %q, want 150%%", got)
	}
}
