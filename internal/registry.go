package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type RegistryImage struct {
	Tag      string
	Archive  string
	SHA256   string
	SizeByte int64
}

func DefaultRegistryDir() string {
	if dir := os.Getenv("DOCKAN_REGISTRY"); dir != "" {
		return dir
	}
	return filepath.Join(StoreRoot(), "registry")
}

func PushImageToRegistry(ref, registryDir string) error {
	if registryDir == "" {
		registryDir = DefaultRegistryDir()
	}
	imagePath, err := ResolveImageReference(ref)
	if err != nil {
		return err
	}
	tag := NormalizeTag(ref)
	meta, _ := ParseMeta(filepath.Join(imagePath, "meta.conf"))
	if meta["tag"] != "" {
		tag = NormalizeTag(meta["tag"])
	}
	imagesDir := filepath.Join(registryDir, "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return err
	}
	archive := filepath.Join(imagesDir, safeTag(tag)+".tar.gz")
	tmp := archive + ".tmp"
	if err := ExportImage(imagePath, tmp); err != nil {
		return err
	}
	sum, size, err := fileSHA256(tmp)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, archive); err != nil {
		return err
	}
	if err := os.WriteFile(archive+".sha256", []byte(sum+"  "+filepath.Base(archive)+"\n"), 0644); err != nil {
		return err
	}
	if err := upsertRegistryIndex(registryDir, RegistryImage{Tag: tag, Archive: filepath.Base(archive), SHA256: sum, SizeByte: size}); err != nil {
		return err
	}
	fmt.Printf("Pushed %s to %s\n", tag, archive)
	return nil
}

func PullImageFromRegistry(ref, registryDir string) error {
	if registryDir == "" {
		registryDir = DefaultRegistryDir()
	}
	tag := NormalizeTag(ref)
	archive := filepath.Join(registryDir, "images", safeTag(tag)+".tar.gz")
	if _, err := os.Stat(archive); err != nil {
		return fmt.Errorf("image introuvable dans registry locale: %s", tag)
	}
	expected, err := readSHA256File(archive + ".sha256")
	if err != nil {
		return err
	}
	actual, _, err := fileSHA256(archive)
	if err != nil {
		return err
	}
	if expected != "" && expected != actual {
		return fmt.Errorf("checksum invalide pour %s", archive)
	}
	target := StoreImagePath(tag)
	tmp := target + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(tmp), 0755); err != nil {
		return err
	}
	if err := ImportImage(archive, tmp); err != nil {
		return err
	}
	meta, _ := ParseMeta(filepath.Join(tmp, "meta.conf"))
	meta["tag"] = tag
	if meta["name"] == "" {
		meta["name"] = strings.Split(tag, ":")[0]
	}
	if err := WriteMeta(filepath.Join(tmp, "meta.conf"), meta); err != nil {
		return err
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	fmt.Printf("Pulled %s from %s\n", tag, archive)
	return nil
}

func ListRegistryImages(registryDir string) ([]RegistryImage, error) {
	if registryDir == "" {
		registryDir = DefaultRegistryDir()
	}
	index, err := readRegistryIndex(registryDir)
	if err != nil {
		return nil, err
	}
	if len(index) > 0 {
		return index, nil
	}
	imagesDir := filepath.Join(registryDir, "images")
	entries, err := os.ReadDir(imagesDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var images []RegistryImage
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}
		path := filepath.Join(imagesDir, entry.Name())
		sum, size, _ := fileSHA256(path)
		images = append(images, RegistryImage{
			Tag:      strings.TrimSuffix(entry.Name(), ".tar.gz"),
			Archive:  entry.Name(),
			SHA256:   sum,
			SizeByte: size,
		})
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Tag < images[j].Tag })
	return images, nil
}

func PrintRegistryImages(registryDir string) error {
	images, err := ListRegistryImages(registryDir)
	if err != nil {
		return err
	}
	fmt.Printf("%-30s %-12s %s\n", "TAG", "SIZE", "SHA256")
	for _, image := range images {
		fmt.Printf("%-30s %-12d %s\n", image.Tag, image.SizeByte, image.SHA256)
	}
	return nil
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func readSHA256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], nil
}

func registryIndexPath(registryDir string) string {
	return filepath.Join(registryDir, "index.tsv")
}

func readRegistryIndex(registryDir string) ([]RegistryImage, error) {
	data, err := os.ReadFile(registryIndexPath(registryDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var images []RegistryImage
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		var size int64
		fmt.Sscanf(parts[3], "%d", &size)
		images = append(images, RegistryImage{Tag: parts[0], Archive: parts[1], SHA256: parts[2], SizeByte: size})
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Tag < images[j].Tag })
	return images, nil
}

func upsertRegistryIndex(registryDir string, image RegistryImage) error {
	images, err := readRegistryIndex(registryDir)
	if err != nil {
		return err
	}
	replaced := false
	for i := range images {
		if images[i].Tag == image.Tag {
			images[i] = image
			replaced = true
			break
		}
	}
	if !replaced {
		images = append(images, image)
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Tag < images[j].Tag })
	var b strings.Builder
	b.WriteString("# tag\tarchive\tsha256\tsize\n")
	for _, item := range images {
		b.WriteString(item.Tag)
		b.WriteByte('\t')
		b.WriteString(item.Archive)
		b.WriteByte('\t')
		b.WriteString(item.SHA256)
		b.WriteByte('\t')
		fmt.Fprintf(&b, "%d\n", item.SizeByte)
	}
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(registryIndexPath(registryDir), []byte(b.String()), 0644)
}
