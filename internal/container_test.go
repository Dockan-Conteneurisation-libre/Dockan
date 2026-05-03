package internal

import "testing"

func TestHostSharedProxyPortsOnlyIncludesTranslatedPorts(t *testing.T) {
	got := hostSharedProxyPorts([]string{"8081:80", "9090:9090", "8443:443"})
	want := []string{"8081:80", "8443:443"}
	if len(got) != len(want) {
		t.Fatalf("hostSharedProxyPorts() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hostSharedProxyPorts() = %#v, want %#v", got, want)
		}
	}
}

func TestHealthcheckCommandParsesCommonForms(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "cmd shell form", raw: "CMD curl -f http://127.0.0.1:8000", want: "curl -f http://127.0.0.1:8000"},
		{name: "cmd shell", raw: "CMD-SHELL test -f /tmp/ready", want: "test -f /tmp/ready"},
		{name: "options before cmd", raw: "--interval=5s --timeout=2s CMD echo ok", want: "echo ok"},
		{name: "exec form", raw: `CMD ["curl", "-f", "http://127.0.0.1:8000"]`, want: `'curl' '-f' 'http://127.0.0.1:8000'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := healthcheckCommand(tt.raw)
			if err != nil {
				t.Fatalf("healthcheckCommand() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("healthcheckCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHealthcheckCommandHandlesNone(t *testing.T) {
	got, err := healthcheckCommand("NONE")
	if err != nil {
		t.Fatalf("healthcheckCommand() error = %v", err)
	}
	if got != "" {
		t.Fatalf("healthcheckCommand() = %q, want empty", got)
	}
}
