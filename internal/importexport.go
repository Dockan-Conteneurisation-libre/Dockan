package internal

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExportImage archive une image Dockan en .tar.gz
func ExportImage(imagePath, outFile string) error {
	f, err := os.Create(outFile)
	if err != nil {
		return err
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	return filepath.Walk(imagePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(filepath.Dir(imagePath), path)
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(tw, f)
			return err
		}
		return nil
	})
}

// ImportImage extrait une archive .tar.gz dans un dossier Dockan
func ImportImage(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		outPath := filepath.Join(destDir, hdr.Name)
		if strings.HasSuffix(hdr.Name, "/") {
			os.MkdirAll(outPath, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(outPath), 0755)
		f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY, os.FileMode(hdr.Mode))
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	return nil
}

// Ajoute l'intégration CLI pour export/import
// Dans cmd/dockan.go, ajoute :
//
// case "export":
//   if len(os.Args) < 4 {
//     fmt.Println("Usage: dockan export <image.dockan> <fichier.tar.gz>")
//     os.Exit(1)
//   }
//   if err := internal.ExportImage(os.Args[2], os.Args[3]); err != nil {
//     fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
//     os.Exit(1)
//   }
// case "import":
//   if len(os.Args) < 4 {
//     fmt.Println("Usage: dockan import <fichier.tar.gz> <dossier.dockan>")
//     os.Exit(1)
//   }
//   if err := internal.ImportImage(os.Args[2], os.Args[3]); err != nil {
//     fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
//     os.Exit(1)
//   }
//
// Cela permet d'utiliser dockan export/import en ligne de commande.
