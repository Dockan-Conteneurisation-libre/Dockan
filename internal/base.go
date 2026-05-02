package internal

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ImportBaseImage(tag, source string) error {
	tag = NormalizeTag(tag)
	target := StoreImagePath(tag)
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	for _, subdir := range []string{"rootfs", "hooks", "volumes"} {
		if err := os.MkdirAll(filepath.Join(target, subdir), 0755); err != nil {
			return err
		}
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		root := source
		if _, err := os.Stat(filepath.Join(source, "rootfs")); err == nil {
			root = filepath.Join(source, "rootfs")
		}
		if err := copyDir(root, filepath.Join(target, "rootfs")); err != nil {
			return err
		}
	} else {
		if err := importRootfsArchive(source, filepath.Join(target, "rootfs")); err != nil {
			return err
		}
	}
	meta := map[string]string{
		"name": strings.Split(tag, ":")[0],
		"tag":  tag,
		"base": "local",
	}
	if err := WriteMeta(filepath.Join(target, "meta.conf"), meta); err != nil {
		return err
	}
	startScript := "#!/usr/bin/env bash\nset -euo pipefail\ncd \"$(dirname \"$0\")/rootfs\"\nexec ${SHELL:-/bin/sh}\n"
	if err := os.WriteFile(filepath.Join(target, "start.sh"), []byte(startScript), 0755); err != nil {
		return err
	}
	fmt.Printf("Imported local base %s from %s\n", tag, source)
	return nil
}

func CreateRuntimeBase(tag, source string) error {
	if _, known, _ := ResolveHostRuntimeBase(tag); !known {
		return fmt.Errorf("runtime non reconnu: %s", tag)
	}
	if err := ImportBaseImage(tag, source); err != nil {
		return err
	}
	target := StoreImagePath(tag)
	meta, _ := ParseMeta(filepath.Join(target, "meta.conf"))
	meta["base"] = "runtime"
	meta["runtime.ref"] = NormalizeTag(tag)
	if runtime, ok, _ := ResolveHostRuntimeBase(tag); ok {
		meta["runtime"] = runtime.Name
		meta["runtime.command"] = runtime.Command
	}
	return WriteMeta(filepath.Join(target, "meta.conf"), meta)
}

func importRootfsArchive(archivePath, rootfs string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := io.Reader(f)
	var gz *gzip.Reader
	if strings.HasSuffix(archivePath, ".gz") || strings.HasSuffix(archivePath, ".tgz") {
		gz, err = gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		reader = gz
	}
	tr := tar.NewReader(reader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		cleanName, err := cleanArchivePath(strings.TrimPrefix(hdr.Name, "rootfs/"))
		if err != nil {
			return err
		}
		if cleanName == "." {
			continue
		}
		outPath := filepath.Join(rootfs, cleanName)
		if hdr.FileInfo().IsDir() || strings.HasSuffix(hdr.Name, "/") {
			if err := os.MkdirAll(outPath, 0755); err != nil {
				return err
			}
			continue
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}
