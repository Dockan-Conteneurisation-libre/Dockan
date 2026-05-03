package internal

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageEnvSetsPortFromPublishedHostPort(t *testing.T) {
	env := imageEnv(&Image{Meta: map[string]string{}, Path: t.TempDir(), RootfsDir: t.TempDir()}, RunOptions{Ports: []string{"18080:8000"}})
	if !containsEnv(env, "PORT=18080") {
		t.Fatalf("PORT should use host port for shared host networking: %v", env)
	}
}

func TestImageEnvDoesNotOverrideExplicitPort(t *testing.T) {
	env := imageEnv(&Image{Meta: map[string]string{}, Path: t.TempDir(), RootfsDir: t.TempDir()}, RunOptions{Ports: []string{"18080:8000"}, Env: []string{"PORT=9000"}})
	if !containsEnv(env, "PORT=9000") || containsEnv(env, "PORT=18080") {
		t.Fatalf("explicit PORT should be preserved: %v", env)
	}
}

func TestCleanHostEnvForContainerRemovesDockanRuntimeEnv(t *testing.T) {
	got := cleanHostEnvForContainer([]string{
		"PATH=/usr/bin",
		"DOCKAN_HOME=/var/lib/dockan",
		"DOCKAN_RUN_COMMAND=frankenphp run",
		"DOCKAN_ENTRYPOINT=/entrypoint.sh",
		"DOCKAN_META_NAME=panel",
		"DOCKAN_VOLUME_DATA=/var/lib/dockan/volumes/data",
	})
	joined := strings.Join(got, "\n")
	for _, bad := range []string{"DOCKAN_RUN_COMMAND=", "DOCKAN_ENTRYPOINT=", "DOCKAN_META_NAME=", "DOCKAN_VOLUME_DATA="} {
		if strings.Contains(joined, bad) {
			t.Fatalf("environment still contains %s in %q", bad, joined)
		}
	}
	if !strings.Contains(joined, "DOCKAN_HOME=/var/lib/dockan") {
		t.Fatalf("DOCKAN_HOME should be preserved: %q", joined)
	}
}

func containsEnv(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}

func TestInitBuildRunExportImport(t *testing.T) {
	base := t.TempDir()
	imagePath := filepath.Join(base, "app.dockan")

	if err := InitImage(imagePath); err != nil {
		t.Fatalf("InitImage() error = %v", err)
	}
	if err := BuildImage(imagePath); err != nil {
		t.Fatalf("BuildImage() error = %v", err)
	}
	if err := RunContainerLifecycle(imagePath, RunOptions{Isolation: IsolationNone}); err != nil {
		t.Fatalf("RunContainerLifecycle() error = %v", err)
	}

	archivePath := filepath.Join(base, "app.tar.gz")
	if err := ExportImage(imagePath, archivePath); err != nil {
		t.Fatalf("ExportImage() error = %v", err)
	}

	importPath := filepath.Join(base, "imported.dockan")
	if err := ImportImage(archivePath, importPath); err != nil {
		t.Fatalf("ImportImage() error = %v", err)
	}
	if _, err := LoadImage(importPath); err != nil {
		t.Fatalf("LoadImage(imported) error = %v", err)
	}
}

func TestImportRejectsPathTraversal(t *testing.T) {
	base := t.TempDir()
	archivePath := filepath.Join(base, "bad.tar.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: "../evil", Mode: 0644, Size: 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if err := ImportImage(archivePath, filepath.Join(base, "out.dockan")); err == nil {
		t.Fatal("ImportImage() expected path traversal error")
	}
}

func TestPrepareVolumesRejectsPathTraversal(t *testing.T) {
	imagePath := filepath.Join(t.TempDir(), "app.dockan")
	if err := InitImage(imagePath); err != nil {
		t.Fatal(err)
	}
	_, err := PrepareVolumes(imagePath, map[string]string{"volumes": "bad:../../escape"})
	if err == nil {
		t.Fatal("PrepareVolumes() expected path traversal error")
	}
}

func TestBuildFromContextCreatesRunnableTaggedImage(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	contextDir := filepath.Join(base, "context")
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "Dockanfile"), []byte("FROM scratch\nLABEL org.opencontainers.image.title=Hello\nCOPY hello.sh /hello.sh\nRUN chmod +x hello.sh\nCMD ./hello.sh\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "hello.sh"), []byte("#!/usr/bin/env sh\necho hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := BuildFromContext(BuildOptions{Tag: "hello:test", Context: contextDir}); err != nil {
		t.Fatalf("BuildFromContext() error = %v", err)
	}
	imagePath, err := ResolveImageReference("hello:test")
	if err != nil {
		t.Fatalf("ResolveImageReference() error = %v", err)
	}
	if _, err := LoadImage(imagePath); err != nil {
		t.Fatalf("LoadImage() error = %v", err)
	}
	if err := RunContainerLifecycle(imagePath, RunOptions{Isolation: IsolationNone}); err != nil {
		t.Fatalf("RunContainerLifecycle() error = %v", err)
	}
}

func TestBuildFromContextUsesLocalDockerfile(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	contextDir := filepath.Join(base, "context")
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte("FROM scratch\nENV MODE=local\nWORKDIR /app\nCOPY app.sh /app/app.sh\nRUN chmod +x app.sh\nCMD ./app.sh\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "app.sh"), []byte("#!/usr/bin/env sh\necho $MODE\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := BuildFromContext(BuildOptions{Tag: "dockerfile:test", Context: contextDir}); err != nil {
		t.Fatalf("BuildFromContext() error = %v", err)
	}
	imagePath, err := ResolveImageReference("dockerfile:test")
	if err != nil {
		t.Fatalf("ResolveImageReference() error = %v", err)
	}
	meta, err := ParseMeta(filepath.Join(imagePath, "meta.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if meta["env"] != "MODE=local" || meta["workdir"] != "/app" {
		t.Fatalf("meta = %#v", meta)
	}
}

func TestBuildFromContextSupportsDockerExecFormCommands(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	contextDir := filepath.Join(base, "context")
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "FROM scratch\nRUN [\"sh\", \"-c\", \"printf ok > marker.txt\"]\nENTRYPOINT [\"echo\"]\nCMD [\"hello world\"]\n"
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := BuildFromContext(BuildOptions{Tag: "execform:test", Context: contextDir}); err != nil {
		t.Fatalf("BuildFromContext() error = %v", err)
	}
	imagePath, err := ResolveImageReference("execform:test")
	if err != nil {
		t.Fatalf("ResolveImageReference() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(imagePath, "rootfs", "marker.txt")); err != nil {
		t.Fatalf("RUN exec form did not create marker: %v", err)
	}
	startScript, err := os.ReadFile(filepath.Join(imagePath, "start.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(startScript), "'echo' 'hello world'") {
		t.Fatalf("start script does not contain normalized exec form: %s", startScript)
	}
}

func TestBuildFromContextUsesHostRuntimeBaseWithoutDockerHub(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	binDir := filepath.Join(base, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "php"), []byte("#!/usr/bin/env sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	contextDir := filepath.Join(base, "context")
	if err := os.MkdirAll(filepath.Join(contextDir, "public"), 0755); err != nil {
		t.Fatal(err)
	}
	content := "FROM php:8.3\nWORKDIR /app\nCOPY public ./public\nEXPOSE 8000\nCMD php -S 0.0.0.0:8000 -t public\n"
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "public", "index.php"), []byte("<?php echo 'ok';\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := BuildFromContext(BuildOptions{Tag: "phphost:test", Context: contextDir}); err != nil {
		t.Fatalf("BuildFromContext() error = %v", err)
	}
	imagePath, err := ResolveImageReference("phphost:test")
	if err != nil {
		t.Fatalf("ResolveImageReference() error = %v", err)
	}
	meta, err := ParseMeta(filepath.Join(imagePath, "meta.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if meta["base"] != "php:8.3" || meta["base.mode"] != "host-runtime" || meta["runtime.command"] != "php" {
		t.Fatalf("meta = %#v", meta)
	}
}

func TestBuildFromContextHostRuntimeBaseRequiresRuntimeCommand(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	t.Setenv("PATH", filepath.Join(base, "empty-bin"))
	contextDir := filepath.Join(base, "context")
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte("FROM node:20\nCMD node server.js\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := BuildFromContext(BuildOptions{Tag: "missingnode:test", Context: contextDir})
	if err == nil {
		t.Fatal("BuildFromContext() expected missing runtime error")
	}
	if !strings.Contains(err.Error(), "node introuvable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildFromContextSupportsDockerfileVolumeAndMetadata(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	contextDir := filepath.Join(base, "context")
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "FROM scratch\nLABEL org.opencontainers.image.title=Demo\nUSER app\nEXPOSE 8080 9000\nVOLUME [\"/data\", \"/cache\"]\nCMD echo ok\n"
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := BuildFromContext(BuildOptions{Tag: "meta:test", Context: contextDir}); err != nil {
		t.Fatalf("BuildFromContext() error = %v", err)
	}
	imagePath, err := ResolveImageReference("meta:test")
	if err != nil {
		t.Fatalf("ResolveImageReference() error = %v", err)
	}
	meta, err := ParseMeta(filepath.Join(imagePath, "meta.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if meta["ports"] != "8080,9000" || meta["volumes"] != "/data,/cache" || meta["user"] != "app" {
		t.Fatalf("meta = %#v", meta)
	}
}

func TestBuildFromContextHonorsDockerignore(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	contextDir := filepath.Join(base, "context")
	if err := os.MkdirAll(filepath.Join(contextDir, "secret"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte("FROM scratch\nCOPY . /app\nCMD echo ok\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, ".dockerignore"), []byte("secret\n*.tmp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "keep.txt"), []byte("ok\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "drop.tmp"), []byte("no\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "secret", "token"), []byte("no\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := BuildFromContext(BuildOptions{Tag: "ignore:test", Context: contextDir}); err != nil {
		t.Fatalf("BuildFromContext() error = %v", err)
	}
	imagePath, err := ResolveImageReference("ignore:test")
	if err != nil {
		t.Fatalf("ResolveImageReference() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(imagePath, "rootfs", "app", "keep.txt")); err != nil {
		t.Fatalf("kept file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(imagePath, "rootfs", "app", "drop.tmp")); !os.IsNotExist(err) {
		t.Fatalf("ignored tmp copied, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(imagePath, "rootfs", "app", "secret", "token")); !os.IsNotExist(err) {
		t.Fatalf("ignored directory copied, err=%v", err)
	}
}

func TestBuildFromContextSupportsBasicMultistageCopy(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	contextDir := filepath.Join(base, "context")
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "FROM scratch AS builder\nCOPY artifact.txt /out/artifact.txt\nFROM scratch\nCOPY --from=builder /out/artifact.txt /app/artifact.txt\nCMD echo ok\n"
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "artifact.txt"), []byte("built\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := BuildFromContext(BuildOptions{Tag: "multi:test", Context: contextDir}); err != nil {
		t.Fatalf("BuildFromContext() error = %v", err)
	}
	imagePath, err := ResolveImageReference("multi:test")
	if err != nil {
		t.Fatalf("ResolveImageReference() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(imagePath, "rootfs", "app", "artifact.txt"))
	if err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
	if string(data) != "built\n" {
		t.Fatalf("artifact = %q", data)
	}
}

func TestPrepareVolumesSupportsAnonymousAndRuntimeVolumes(t *testing.T) {
	base := t.TempDir()
	imagePath := filepath.Join(base, "app.dockan")
	if err := InitImage(imagePath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	hostDir := filepath.Join(base, "host-data")
	cleanup, err := PrepareVolumesForRun(imagePath, map[string]string{"volumes": "/data"}, []string{hostDir + ":/cache"})
	if err != nil {
		t.Fatalf("PrepareVolumesForRun() error = %v", err)
	}
	defer cleanup()
	if _, err := os.Stat(filepath.Join(imagePath, "volumes", "data")); err != nil {
		t.Fatalf("anonymous volume missing: %v", err)
	}
	if _, err := os.Stat(hostDir); err != nil {
		t.Fatalf("runtime host volume missing: %v", err)
	}
}

func TestImportBaseImageFromLocalRootfs(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	rootfs := filepath.Join(base, "rootfs")
	if err := os.MkdirAll(filepath.Join(rootfs, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootfs, "etc-release"), []byte("local\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ImportBaseImage("localbase:test", rootfs); err != nil {
		t.Fatalf("ImportBaseImage() error = %v", err)
	}
	imagePath, err := ResolveImageReference("localbase:test")
	if err != nil {
		t.Fatalf("ResolveImageReference() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(imagePath, "rootfs", "etc-release")); err != nil {
		t.Fatalf("imported rootfs missing file: %v", err)
	}
}

func TestCreateRuntimeBaseFromLocalRootfs(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DOCKAN_HOME", filepath.Join(base, "store"))
	rootfs := filepath.Join(base, "rootfs")
	if err := os.MkdirAll(filepath.Join(rootfs, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootfs, "bin", "php"), []byte("#!/usr/bin/env sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := CreateRuntimeBase("php:8.3", rootfs); err != nil {
		t.Fatalf("CreateRuntimeBase() error = %v", err)
	}
	imagePath, err := ResolveImageReference("php:8.3")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ParseMeta(filepath.Join(imagePath, "meta.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if meta["base"] != "runtime" || meta["runtime.ref"] != "php:8.3" {
		t.Fatalf("meta = %#v", meta)
	}
}
