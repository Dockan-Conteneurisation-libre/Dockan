package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPushPullLocalRegistry(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	contextDir := filepath.Join(base, "context")
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "Dockanfile"), []byte("FROM scratch\nCOPY app.sh /app.sh\nCMD ./app.sh\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "app.sh"), []byte("#!/usr/bin/env sh\necho registry\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := BuildFromContext(BuildOptions{Tag: "registry:test", Context: contextDir}); err != nil {
		t.Fatalf("BuildFromContext() error = %v", err)
	}
	registryDir := filepath.Join(base, "registry")
	if err := PushImageToRegistry("registry:test", registryDir); err != nil {
		t.Fatalf("PushImageToRegistry() error = %v", err)
	}
	if err := RemoveImage("registry:test"); err != nil {
		t.Fatalf("RemoveImage() error = %v", err)
	}
	if err := PullImageFromRegistry("registry:test", registryDir); err != nil {
		t.Fatalf("PullImageFromRegistry() error = %v", err)
	}
	if _, err := ResolveImageReference("registry:test"); err != nil {
		t.Fatalf("ResolveImageReference() after pull error = %v", err)
	}
	images, err := ListRegistryImages(registryDir)
	if err != nil {
		t.Fatalf("ListRegistryImages() error = %v", err)
	}
	if len(images) != 1 || images[0].Tag != "registry:test" || images[0].SHA256 == "" {
		t.Fatalf("images = %#v", images)
	}
}

func TestPullLocalRegistryRejectsBadChecksum(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	registryDir := filepath.Join(base, "registry")
	imagesDir := filepath.Join(registryDir, "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(imagesDir, safeTag("bad:test")+".tar.gz")
	if err := os.WriteFile(archive, []byte("bad"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive+".sha256", []byte("0000  bad_test.tar.gz\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := PullImageFromRegistry("bad:test", registryDir); err == nil {
		t.Fatal("PullImageFromRegistry() expected checksum error")
	}
}
