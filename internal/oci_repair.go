package internal

import (
	"os"
	"path/filepath"
	"strings"
)

func RepairOCIRootfs(img *Image) error {
	if img.Meta["rootfs.mode"] != "oci" {
		return nil
	}
	for _, link := range []struct {
		path   string
		target string
		marker string
	}{
		{path: "bin", target: "usr/bin", marker: "bin.usr-is-merged"},
		{path: "sbin", target: "usr/sbin", marker: "sbin.usr-is-merged"},
		{path: "lib", target: "usr/lib", marker: "lib.usr-is-merged"},
		{path: "lib64", target: "usr/lib64", marker: "lib64.usr-is-merged"},
	} {
		if err := ensureOCIMergedUsrLink(img.RootfsDir, link.path, link.target, link.marker); err != nil {
			return err
		}
	}
	if workdir := strings.TrimSpace(img.Meta["workdir"]); workdir != "" && workdir != "/" {
		target := filepath.Join(img.RootfsDir, strings.TrimPrefix(filepath.Clean(workdir), string(filepath.Separator)))
		if err := os.MkdirAll(target, 0755); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(img.RootfsDir, "tmp"), 01777); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(img.RootfsDir, "tmp"), 01777); err != nil {
		return err
	}
	return nil
}

func ensureOCIMergedUsrLink(rootfs, path, target, marker string) error {
	fullPath := filepath.Join(rootfs, path)
	if _, err := os.Lstat(fullPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(filepath.Join(rootfs, marker)); err != nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(rootfs, target)); err != nil {
		return nil
	}
	return os.Symlink(target, fullPath)
}
