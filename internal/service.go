package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ServiceOptions struct {
	File string
	Name string
	User bool
}

func InstallService(opts ServiceOptions) error {
	opts = normalizeServiceOptions(opts)
	if err := validateResourceName("nom de service", opts.Name); err != nil {
		return err
	}
	if _, err := LoadComposeFile(opts.File); err != nil {
		return err
	}
	unitPath, err := serviceUnitPath(opts)
	if err != nil {
		return err
	}
	dockanBin, err := os.Executable()
	if err != nil {
		return err
	}
	fileAbs := absOrSame(opts.File)
	unit := serviceUnitContent(opts, dockanBin, fileAbs)
	if err := os.MkdirAll(filepath.Dir(unitPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, []byte(unit), 0644); err != nil {
		return err
	}
	fmt.Printf("Service installé: %s\n", unitPath)
	if opts.User {
		fmt.Println("Activer: systemctl --user daemon-reload && systemctl --user enable --now " + filepath.Base(unitPath))
		return nil
	}
	fmt.Println("Activer: sudo systemctl daemon-reload && sudo systemctl enable --now " + filepath.Base(unitPath))
	return nil
}

func UninstallService(opts ServiceOptions) error {
	opts = normalizeServiceOptions(opts)
	if err := validateResourceName("nom de service", opts.Name); err != nil {
		return err
	}
	unitPath, err := serviceUnitPath(opts)
	if err != nil {
		return err
	}
	if err := os.Remove(unitPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("service introuvable: %s", unitPath)
		}
		return err
	}
	fmt.Printf("Service supprimé: %s\n", unitPath)
	return nil
}

func normalizeServiceOptions(opts ServiceOptions) ServiceOptions {
	if opts.File == "" {
		opts.File = "dockan.yml"
	}
	if opts.Name == "" {
		project, err := LoadComposeFile(opts.File)
		if err == nil && project.Name != "" {
			opts.Name = project.Name
		} else {
			opts.Name = safeTag(strings.TrimSuffix(filepath.Base(filepath.Dir(absOrSame(opts.File))), ".dockan"))
		}
	}
	return opts
}

func serviceUnitPath(opts ServiceOptions) (string, error) {
	fileName := "dockan-" + opts.Name + ".service"
	if opts.User {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", "systemd", "user", fileName), nil
	}
	return filepath.Join("/etc", "systemd", "system", fileName), nil
}

func serviceUnitContent(opts ServiceOptions, dockanBin, fileAbs string) string {
	wantedBy := "multi-user.target"
	if opts.User {
		wantedBy = "default.target"
	}
	var envLine string
	if dockanHome := os.Getenv("DOCKAN_HOME"); dockanHome != "" {
		envLine = "Environment=DOCKAN_HOME=" + unitQuote(dockanHome) + "\n"
	}
	return fmt.Sprintf(`[Unit]
Description=Dockan project %s
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=%s
%sExecStart=%s compose up -f %s
ExecStop=%s compose down -f %s
Restart=on-failure
RestartSec=5s
TimeoutStartSec=0

[Install]
WantedBy=%s
`, opts.Name, unitPath(filepath.Dir(fileAbs)), envLine, unitQuote(dockanBin), unitQuote(fileAbs), unitQuote(dockanBin), unitQuote(fileAbs), wantedBy)
}

func unitPath(value string) string {
	return strings.NewReplacer(
		`%`, `%%`,
		`\`, `\\`,
		` `, `\x20`,
		"\t", `\x09`,
		"\n", `\x0a`,
		"\r", `\x0d`,
	).Replace(value)
}

func unitQuote(value string) string {
	return strconv.Quote(value)
}
