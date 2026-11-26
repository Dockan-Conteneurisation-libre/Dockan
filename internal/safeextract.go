package internal

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

// maxSymlinkHops bounds symlink resolution so that a crafted archive cannot
// trap the extractor in a loop.
const maxSymlinkHops = 255

// secureJoin resolves unsafePath inside root the way the kernel would if root
// were "/" (chroot semantics): absolute symlink targets restart at root and
// ".." never climbs above root. The result always lies within root, so a
// symlink planted by an archive entry can never redirect a later write or
// removal onto the host filesystem.
func secureJoin(root, unsafePath string) (string, error) {
	root = filepath.Clean(root)
	remaining := filepath.ToSlash(unsafePath)
	resolved := ""
	hops := 0
	for remaining != "" {
		part := remaining
		remaining = ""
		if i := strings.IndexByte(part, '/'); i >= 0 {
			part, remaining = part[:i], part[i+1:]
		}
		switch part {
		case "", ".":
			continue
		case "..":
			resolved = parentWithinRoot(resolved)
			continue
		}
		next := path.Join(resolved, part)
		link, isLink, err := readLinkIfAny(filepath.Join(root, filepath.FromSlash(next)))
		if err != nil {
			return "", err
		}
		if !isLink {
			resolved = next
			continue
		}
		hops++
		if hops > maxSymlinkHops {
			return "", fmt.Errorf("trop de liens symboliques en résolvant %s", unsafePath)
		}
		if path.IsAbs(filepath.ToSlash(link)) {
			resolved = ""
		}
		remaining = filepath.ToSlash(link) + "/" + remaining
	}
	return filepath.Join(root, filepath.FromSlash(resolved)), nil
}

func parentWithinRoot(resolved string) string {
	parent := path.Dir(resolved)
	if parent == "." || parent == "/" {
		return ""
	}
	return parent
}

// readLinkIfAny reports whether p is a symlink and returns its target.
// A missing path is not an error: archive entries may create it later.
func readLinkIfAny(p string) (string, bool, error) {
	info, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return "", false, nil
	}
	link, err := os.Readlink(p)
	if err != nil {
		return "", false, err
	}
	return link, true, nil
}

// archiveEntryPath returns where an archive entry lands inside root. Every
// parent component is resolved with secureJoin; the last component is kept
// as is so that the entry replaces a symlink instead of following it.
func archiveEntryPath(root, name string) (string, error) {
	clean := path.Clean("/" + filepath.ToSlash(name))
	if clean == "/" {
		return "", fmt.Errorf("entrée d'archive sans nom: %q", name)
	}
	parent, err := secureJoin(root, path.Dir(clean))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, path.Base(clean)), nil
}

// removeExistingEntry deletes whatever occupies target, a directory tree
// included, as a later layer replacing a path does. os.RemoveAll never follows
// a symlink found at target: it removes the link itself.
func removeExistingEntry(target string) error {
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("impossible de remplacer %s: %w", target, err)
	}
	return nil
}

// writeArchiveFile creates target as a fresh regular file, never following a
// symlink left at that path. With exactMode the permissions are applied
// regardless of the umask, as layer extraction requires.
func writeArchiveFile(target string, r io.Reader, mode os.FileMode, exactMode bool) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	if err := removeExistingEntry(target); err != nil {
		return err
	}
	flags := os.O_CREATE | os.O_EXCL | os.O_WRONLY | syscall.O_NOFOLLOW
	f, err := os.OpenFile(target, flags, mode|0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	if exactMode {
		if err := f.Chmod(mode); err != nil {
			f.Close()
			return err
		}
	}
	return f.Close()
}
