package internal

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Image représente une image Dockan (manifest, rootfs, etc.)
type Image struct {
	Path      string
	Meta      map[string]string
	RootfsDir string
}

// LoadImage charge une image Dockan depuis un dossier
func LoadImage(path string) (*Image, error) {
	meta, err := ParseMeta(filepath.Join(path, "meta.conf"))
	if err != nil {
		return nil, err
	}
	rootfs := filepath.Join(path, "rootfs")
	if _, err := os.Stat(rootfs); err != nil {
		return nil, fmt.Errorf("rootfs/ manquant dans %s", path)
	}
	return &Image{
		Path:      path,
		Meta:      meta,
		RootfsDir: rootfs,
	}, nil
}

func RunImage(path string) error {
	img, err := LoadImage(path)
	if err != nil {
		return err
	}
	isolation := DetectIsolation()
	if isolation == "" {
		return fmt.Errorf("Aucune méthode d'isolation disponible (firejail, systemd-nspawn, chroot)")
	}
	fmt.Printf("[dockan] Isolation utilisée : %s\n", isolation)
	// Utilise les infos de l'image Dockan pour lancer le conteneur
	startScript := filepath.Join(img.Path, "start.sh")
	rootfs := img.RootfsDir
	stdout, stderr, closeLog, err := LogWriters(img.Path)
	if err != nil {
		return err
	}
	defer closeLog()
	return RunWithIsolationWithLogs(isolation, rootfs, startScript, stdout, stderr)
}

// RunWithIsolationWithLogs exécute le conteneur avec redirection des logs
func RunWithIsolationWithLogs(isolation, rootfs, startScript string, stdout, stderr io.Writer) error {
	var cmd *exec.Cmd
	switch isolation {
	case IsolationFirejail:
		cmd = exec.Command("firejail", "--quiet", "--private="+rootfs, "--shell="+startScript)
	case IsolationSystemdNspawn:
		cmd = exec.Command("systemd-nspawn", "-D", rootfs, "/start.sh")
	case IsolationChroot:
		cmd = exec.Command("sudo", "chroot", rootfs, "/start.sh")
	default:
		return fmt.Errorf("Aucune méthode d'isolation valide")
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func BuildImage(path string) error {
	buildScript := filepath.Join(path, "build.sh")
	if _, err := os.Stat(buildScript); err != nil {
		return fmt.Errorf("build.sh manquant dans %s", path)
	}
	cmd := exec.Command("bash", buildScript)
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ListImages(base string) {
	filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() && filepath.Ext(path) == ".dockan" {
			meta := filepath.Join(path, "meta.conf")
			name := filepath.Base(path)
			if _, err := os.Stat(meta); err == nil {
				m, _ := ParseMeta(meta)
				if n, ok := m["name"]; ok {
					name = n
				}
			}
			fmt.Printf("%s : %s\n", name, path)
		}
		return nil
	})
}

func InitImage(dir string) error {
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s existe déjà", dir)
	}
	os.MkdirAll(filepath.Join(dir, "rootfs"), 0755)
	os.MkdirAll(filepath.Join(dir, "hooks"), 0755)
	os.MkdirAll(filepath.Join(dir, "volumes"), 0755)
	write := func(p, c string) {
		os.WriteFile(filepath.Join(dir, p), []byte(c), 0755)
	}
	write("build.sh", "#!/bin/bash\necho '(build.sh) Ajoute tes instructions ici.'\n")
	write("start.sh", "#!/bin/bash\necho '(start.sh) Hello depuis Dockan !'\n")
	write("meta.conf", "# Métadonnées Dockan\nname=MonApp\nport=8080\nrequires=bash\n")
	return nil
}

func ParseMeta(path string) (map[string]string, error) {
	meta := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return meta, err
	}
	lines := string(data)
	for _, l := range splitLines(lines) {
		if len(l) == 0 || l[0] == '#' {
			continue
		}
		if i := indexOf(l, '='); i > 0 {
			k := l[:i]
			v := l[i+1:]
			meta[k] = v
		}
	}
	return meta, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := range s {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// ...Isolation et autres fonctions déjà présentes...
