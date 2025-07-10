package main

import (
	"dockan/internal"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Dockan - alternative libre à Docker\nUsage: dockan <commande> [options]\nEssayez 'dockan help'")
		os.Exit(1)
	}
	cmd := os.Args[1]
	switch cmd {
	case "help", "--help", "-h":
		printHelp()
	case "run":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dockan run <image.dockan>")
			os.Exit(1)
		}
		imagePath := os.Args[2]
		if err := internal.RunContainerLifecycle(imagePath); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "build":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dockan build <image.dockan>")
			os.Exit(1)
		}
		imagePath := os.Args[2]
		if err := internal.BuildImage(imagePath); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "list":
		base := "."
		if len(os.Args) > 2 {
			base = os.Args[2]
		}
		internal.ListImages(base)
	case "init":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dockan init <image.dockan>")
			os.Exit(1)
		}
		imagePath := os.Args[2]
		if err := internal.InitImage(imagePath); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "export":
		if len(os.Args) < 4 {
			fmt.Println("Usage: dockan export <image.dockan> <fichier.tar.gz>")
			os.Exit(1)
		}
		if err := internal.ExportImage(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	case "import":
		if len(os.Args) < 4 {
			fmt.Println("Usage: dockan import <fichier.tar.gz> <dossier.dockan>")
			os.Exit(1)
		}
		if err := internal.ImportImage(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Commande inconnue : %s\n", cmd)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`Dockan - alternative libre à Docker
Commandes :
  run <image.dockan>   Lance un conteneur
  build <image.dockan> Construit une image
  list                 Liste les images Dockan
  init <image.dockan>  Crée un squelette d'image
  export <image.dockan> <fichier.tar.gz>  Exporte une image vers un fichier
  import <fichier.tar.gz> <dossier.dockan>  Importe une image depuis un fichier
  help                 Affiche cette aide
`)
}
