package internal

import (
	"reflect"
	"testing"
)

func TestExpandDepsPackagesCoreByManager(t *testing.T) {
	tests := []struct {
		manager string
		want    []string
	}{
		{manager: "apt", want: []string{"bash", "curl", "ca-certificates", "tar", "gzip", "xz-utils", "util-linux", "iproute2", "iptables", "procps"}},
		{manager: "dnf", want: []string{"bash", "curl", "ca-certificates", "tar", "gzip", "xz", "util-linux", "iproute", "iptables", "procps-ng"}},
		{manager: "apk", want: []string{"bash", "curl", "ca-certificates", "tar", "gzip", "xz", "util-linux", "iproute2", "iptables", "procps"}},
		{manager: "pacman", want: []string{"bash", "curl", "ca-certificates", "tar", "gzip", "xz", "util-linux", "iproute2", "iptables", "procps-ng"}},
		{manager: "zypper", want: []string{"bash", "curl", "ca-certificates", "tar", "gzip", "xz", "util-linux", "iproute2", "iptables", "procps"}},
	}
	for _, tt := range tests {
		got, err := expandDepsPackages([]string{"core"}, tt.manager)
		if err != nil {
			t.Fatalf("expandDepsPackages(core, %q) error = %v", tt.manager, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("expandDepsPackages(core, %q) = %#v, want %#v", tt.manager, got, tt.want)
		}
	}
}

func TestExpandDepsPackagesProfilesAndCustomPackages(t *testing.T) {
	got, err := expandDepsPackages([]string{"core", "curl", "git", "@network", "db"}, "dnf")
	if err != nil {
		t.Fatalf("expandDepsPackages() error = %v", err)
	}
	want := []string{"bash", "curl", "ca-certificates", "tar", "gzip", "xz", "util-linux", "iproute", "iptables", "procps-ng", "git", "iputils", "bind-utils", "nmap-ncat", "socat", "sqlite", "mariadb", "postgresql", "redis"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expandDepsPackages() = %#v, want %#v", got, want)
	}
}

func TestExpandDepsPackagesToolsByManager(t *testing.T) {
	got, err := expandDepsPackages([]string{"tools"}, "dnf")
	if err != nil {
		t.Fatalf("expandDepsPackages(tools) error = %v", err)
	}
	for _, want := range []string{"git", "jq", "rsync", "unzip", "zip", "openssl", "gnupg2", "lsof", "file", "findutils", "which"} {
		if !containsString(got, want) {
			t.Fatalf("expandDepsPackages(tools) missing %q in %#v", want, got)
		}
	}
}

func TestExpandDepsPackagesFrontendByManager(t *testing.T) {
	for _, manager := range []string{"apt", "dnf", "apk", "pacman", "zypper"} {
		got, err := expandDepsPackages([]string{"frontend"}, manager)
		if err != nil {
			t.Fatalf("expandDepsPackages(frontend, %q) error = %v", manager, err)
		}
		want := []string{"nodejs", "npm"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("expandDepsPackages(frontend, %q) = %#v, want %#v", manager, got, want)
		}
	}
}

func TestExpandDepsPackagesKeepsExplicitVersionSpecs(t *testing.T) {
	got, err := expandDepsPackages([]string{"nodejs-20.11.1", "php-cli=8.3.6", "nginx=1.26.1-r0"}, "dnf")
	if err != nil {
		t.Fatalf("expandDepsPackages(version specs) error = %v", err)
	}
	want := []string{"nodejs-20.11.1", "php-cli=8.3.6", "nginx=1.26.1-r0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expandDepsPackages(version specs) = %#v, want %#v", got, want)
	}
}

func TestExpandDepsPackagesDatabaseByManager(t *testing.T) {
	tests := []struct {
		manager string
		want    []string
	}{
		{manager: "apt", want: []string{"sqlite3", "mariadb-client", "postgresql-client", "redis-tools"}},
		{manager: "dnf", want: []string{"sqlite", "mariadb", "postgresql", "redis"}},
		{manager: "apk", want: []string{"sqlite", "mariadb-client", "postgresql-client", "redis"}},
		{manager: "pacman", want: []string{"sqlite", "mariadb-clients", "postgresql", "redis"}},
		{manager: "zypper", want: []string{"sqlite3", "mariadb-client", "postgresql", "redis"}},
	}
	for _, tt := range tests {
		got, err := expandDepsPackages([]string{"database"}, tt.manager)
		if err != nil {
			t.Fatalf("expandDepsPackages(database, %q) error = %v", tt.manager, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("expandDepsPackages(database, %q) = %#v, want %#v", tt.manager, got, tt.want)
		}
	}
}

func TestExpandDepsPackagesFullIncludesIsolation(t *testing.T) {
	got, err := expandDepsPackages([]string{"full"}, "apt")
	if err != nil {
		t.Fatalf("expandDepsPackages(full) error = %v", err)
	}
	for _, want := range []string{"bash", "iproute2", "jq", "nodejs", "npm", "sqlite3", "nginx", "caddy", "make", "strace", "firejail", "bubblewrap", "systemd-container"} {
		if !containsString(got, want) {
			t.Fatalf("expandDepsPackages(full) missing %q in %#v", want, got)
		}
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
