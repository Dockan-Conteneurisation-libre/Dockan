package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepairOCIRootfsMergedUsrAndWorkdir(t *testing.T) {
	rootfs := t.TempDir()
	for _, dir := range []string{"usr/bin", "usr/sbin", "usr/lib", "bin.usr-is-merged", "sbin.usr-is-merged", "lib.usr-is-merged"} {
		if err := os.MkdirAll(filepath.Join(rootfs, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	img := &Image{
		RootfsDir: rootfs,
		Meta: map[string]string{
			"rootfs.mode": "oci",
			"workdir":     "/var/www/html",
		},
	}

	if err := RepairOCIRootfs(img); err != nil {
		t.Fatalf("RepairOCIRootfs() error = %v", err)
	}
	for _, link := range []string{"bin", "sbin", "lib"} {
		info, err := os.Lstat(filepath.Join(rootfs, link))
		if err != nil {
			t.Fatalf("%s missing: %v", link, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s is not a symlink", link)
		}
	}
	if _, err := os.Stat(filepath.Join(rootfs, "var/www/html")); err != nil {
		t.Fatalf("workdir missing: %v", err)
	}
	info, err := os.Stat(filepath.Join(rootfs, "tmp"))
	if err != nil {
		t.Fatalf("tmp missing: %v", err)
	}
	if info.Mode().Perm() != 0777 {
		t.Fatalf("tmp mode = %v, want 0777 permissions", info.Mode())
	}
}
