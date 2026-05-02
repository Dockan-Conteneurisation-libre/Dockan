package internal

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupRestoreVolume(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))

	if err := CreateVolume("data"); err != nil {
		t.Fatalf("CreateVolume() error = %v", err)
	}
	sourceFile := filepath.Join(VolumesDir(), "data", "nested", "file.txt")
	if err := os.MkdirAll(filepath.Dir(sourceFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourceFile, []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(base, "data.tar.gz")
	if err := BackupVolume("data", archivePath); err != nil {
		t.Fatalf("BackupVolume() error = %v", err)
	}
	if err := RestoreVolume("restored", archivePath); err != nil {
		t.Fatalf("RestoreVolume() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(VolumesDir(), "restored", "nested", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("restored file = %q", data)
	}
}

func TestRestoreVolumeRejectsPathTraversal(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	archivePath := filepath.Join(base, "bad.tar.gz")
	if err := writeTarGzFile(archivePath, "../evil", "x"); err != nil {
		t.Fatal(err)
	}

	err := RestoreVolume("restored", archivePath)
	if err == nil || !strings.Contains(err.Error(), "chemin archive interdit") {
		t.Fatalf("RestoreVolume() expected path traversal error, got %v", err)
	}
}

func TestRestoreVolumeRequiresEmptyTarget(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	if err := CreateVolume("data"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(VolumesDir(), "data", "existing.txt"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(base, "data.tar.gz")
	if err := writeTarGzFile(archivePath, "new.txt", "new"); err != nil {
		t.Fatal(err)
	}

	err := RestoreVolume("data", archivePath)
	if err == nil || !strings.Contains(err.Error(), "volume non vide") {
		t.Fatalf("RestoreVolume() expected non-empty target error, got %v", err)
	}
}

func writeTarGzFile(path, name, content string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	_, err = tw.Write([]byte(content))
	return err
}
