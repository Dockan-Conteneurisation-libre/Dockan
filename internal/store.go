package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type StoredImage struct {
	Tag  string
	Path string
	Name string
}

const SystemStoreRoot = "/var/lib/dockan"

type StoreScope struct {
	Label string
	Root  string
}

func StoreRoot() string {
	if home := os.Getenv("DOCKAN_HOME"); home != "" {
		return home
	}
	if os.Geteuid() == 0 {
		return SystemStoreRoot
	}
	return UserStoreRoot()
}

func UserStoreRoot() string {
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		return filepath.Join(dataHome, "dockan")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share", "dockan")
	}
	return filepath.Join(".", ".dockan-store")
}

func StoreScopes(scope string) []StoreScope {
	switch scope {
	case "system":
		return []StoreScope{{Label: "system", Root: SystemStoreRoot}}
	case "user":
		return []StoreScope{{Label: "user", Root: UserStoreRoot()}}
	case "all":
		current := StoreRoot()
		scopes := []StoreScope{
			{Label: "current", Root: current},
			{Label: "system", Root: SystemStoreRoot},
			{Label: "user", Root: UserStoreRoot()},
		}
		seen := map[string]bool{}
		var unique []StoreScope
		for _, item := range scopes {
			if seen[item.Root] {
				continue
			}
			seen[item.Root] = true
			unique = append(unique, item)
		}
		return unique
	default:
		return []StoreScope{{Label: "current", Root: StoreRoot()}}
	}
}

func ImagesDir() string {
	return filepath.Join(StoreRoot(), "images")
}

func NormalizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "latest:latest"
	}
	if !strings.Contains(tag, ":") {
		return tag + ":latest"
	}
	return tag
}

func StoreImagePath(tag string) string {
	return filepath.Join(ImagesDir(), safeTag(NormalizeTag(tag))+".dockan")
}

func ResolveImageReference(ref string) (string, error) {
	if _, err := os.Stat(ref); err == nil {
		return ref, nil
	}
	path := StoreImagePath(ref)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("image introuvable: %s", ref)
}

func ListStoredImages() ([]StoredImage, error) {
	dir := ImagesDir()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var images []StoredImage
	for _, entry := range entries {
		if !entry.IsDir() || filepath.Ext(entry.Name()) != ".dockan" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		meta, _ := ParseMeta(filepath.Join(path, "meta.conf"))
		tag := meta["tag"]
		if tag == "" {
			tag = strings.TrimSuffix(entry.Name(), ".dockan")
		}
		name := meta["name"]
		if name == "" {
			name = tag
		}
		images = append(images, StoredImage{Tag: tag, Path: path, Name: name})
	}
	return images, nil
}

func PrintStoredImages() error {
	images, err := ListStoredImages()
	if err != nil {
		return err
	}
	fmt.Printf("%-30s %-24s %s\n", "TAG", "NAME", "PATH")
	for _, image := range images {
		fmt.Printf("%-30s %-24s %s\n", image.Tag, image.Name, image.Path)
	}
	return nil
}

func TagImage(sourceRef, targetTag string) error {
	sourcePath, err := ResolveImageReference(sourceRef)
	if err != nil {
		return err
	}
	targetTag = NormalizeTag(targetTag)
	targetPath := StoreImagePath(targetTag)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}
	if err := copyDir(sourcePath, targetPath); err != nil {
		return err
	}
	meta, _ := ParseMeta(filepath.Join(targetPath, "meta.conf"))
	meta["tag"] = targetTag
	if meta["name"] == "" {
		meta["name"] = strings.Split(targetTag, ":")[0]
	}
	if err := WriteMeta(filepath.Join(targetPath, "meta.conf"), meta); err != nil {
		return err
	}
	fmt.Printf("Tagged %s as %s\n", sourceRef, targetTag)
	return nil
}

func RemoveImage(tag string) error {
	path := StoreImagePath(tag)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("image introuvable: %s", tag)
	}
	return os.RemoveAll(path)
}

func safeTag(tag string) string {
	var b strings.Builder
	for _, r := range tag {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func copyDir(source, target string) error {
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(target, info.Mode())
		}
		outPath := filepath.Join(target, rel)
		if info.IsDir() {
			return os.MkdirAll(outPath, info.Mode())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, outPath, info.Mode())
	})
}

func copyFile(source, target string, mode os.FileMode) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	return os.WriteFile(target, data, mode)
}
