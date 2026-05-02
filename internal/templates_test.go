package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateAppTemplateCreatesPHPProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "phpapp")
	if err := CreateAppTemplate(AppTemplateOptions{Language: "php", Dir: dir, Name: "demo"}); err != nil {
		t.Fatalf("CreateAppTemplate() error = %v", err)
	}
	dockanfile, err := os.ReadFile(filepath.Join(dir, "Dockanfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dockanfile), "FROM php:8.3") || !strings.Contains(string(dockanfile), "EXPOSE 8000") {
		t.Fatalf("unexpected Dockanfile: %s", dockanfile)
	}
	if _, err := os.Stat(filepath.Join(dir, "public", "index.php")); err != nil {
		t.Fatalf("PHP source missing: %v", err)
	}
}

func TestCreateAppTemplateRefusesUnknownLanguage(t *testing.T) {
	err := CreateAppTemplate(AppTemplateOptions{Language: "brainfuck", Dir: t.TempDir()})
	if err == nil {
		t.Fatal("CreateAppTemplate() expected error")
	}
	if !strings.Contains(err.Error(), "template inconnue") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateAppTemplateProtectsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockanfile"), []byte("keep\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := CreateAppTemplate(AppTemplateOptions{Language: "node", Dir: dir})
	if err == nil {
		t.Fatal("CreateAppTemplate() expected existing-file error")
	}
}
