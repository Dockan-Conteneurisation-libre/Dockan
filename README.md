<!-- logo Dockan centré -->
<p align="center">
  <img src="docs/dockan-logo.svg" alt="Dockan logo" width="120" height="120">
</p>

<p align="center">
  <a href="https://github.com/Dockan-Conteneurisation-libre/Dockan/actions">
    <img src="https://github.com/Dockan-Conteneurisation-libre/Dockan/actions/workflows/ci.yml/badge.svg" alt="Build Status">
  </a>
  <a href="https://www.gnu.org/licenses/agpl-3.0.html">
    <img src="https://img.shields.io/badge/Licence-AGPL--3.0-green.svg" alt="Licence: AGPL-3.0">
  </a>
</p>

# Dockan

Dockan est une alternative **libre**, **décentralisée** et **sans daemon** à Docker, conçue pour l’auto-hébergement, l’expérimentation et la simplicité.

## Objectifs
- Isolation légère (firejail, chroot, systemd-nspawn, bubblewrap)
- Format d’image ouvert (dossier ou archive .dockan)
- Pas de registre centralisé
- Pas de daemon
- Facile à packager, partager, auditer

---

## Installation

### Prérequis
- Go >= 1.21 (`sudo apt install golang`)
- Un outil d’isolation : `firejail`, `systemd-nspawn` ou `chroot` (ex : `sudo apt install firejail`)

### Installation rapide
```bash
git clone <repo> dockan
cd dockan
go build -o dockan ./cmd/dockan.go
sudo mv dockan /usr/local/bin/
dockan help
```

---

## Créer une image Dockan pour une application

1. **Générer le squelette**
   ```bash
   dockan init monapp.dockan
   ```
2. **Placer l’application dans `rootfs/`**
   - Exemple Python : copie `app.py` dans `monapp.dockan/rootfs/`
   - Adapter `start.sh` :
     ```bash
     #!/bin/bash
     cd "$(dirname "$0")/rootfs"
     python3 app.py
     ```
3. **Personnaliser `meta.conf`**
   ```properties
   name=MonApp
   port=8080
   requires=python3
   volumes=data:/data
   ```
4. **(Optionnel) Ajouter des hooks**
   - Place un script `prestart` ou `poststop` dans `monapp.dockan/hooks/`
5. **(Optionnel) Build l’image**
   ```bash
   dockan build monapp.dockan
   ```

---

## Commandes principales

- `dockan run <image.dockan>`   : Lance le conteneur
- `dockan build <image.dockan>` : Construit l’image (exécute build.sh)
- `dockan list`                 : Liste les images Dockan
- `dockan init <image.dockan>`  : Crée un squelette d’image
- `dockan export <image.dockan> <fichier.tar.gz>` : Exporte une image
- `dockan import <fichier.tar.gz> <dossier.dockan>` : Importe une image
- `dockan help`                 : Affiche l’aide

---

## Format d’une image Dockan
```
monapp.dockan/
  meta.conf      # Métadonnées (name, port, requires, volumes...)
  build.sh       # Script de build (optionnel)
  start.sh       # Script de démarrage (obligatoire)
  rootfs/        # Racine du conteneur (placez votre app ici)
  hooks/         # Hooks (prestart, poststop, etc.)
  volumes/       # Volumes persistants
  logs/          # Logs d’exécution
```

---

## Exemples

### Exemple d’image Python
```
monapp.dockan/
  meta.conf
  build.sh
  start.sh
  rootfs/
    app.py
```
- `meta.conf` :
  ```properties
  name=MonApp
  port=8080
  requires=python3
  ```
- `start.sh` :
  ```bash
  #!/bin/bash
  cd "$(dirname "$0")/rootfs"
  python3 app.py
  ```

### Exporter/Importer une image
```bash
dockan export monapp.dockan monapp.tar.gz
dockan import monapp.tar.gz monapp.dockan
```

---

## Fonctionnalités avancées
- **Hooks** : scripts dans `hooks/` exécutés avant/après le conteneur (`prestart`, `poststop`)
- **Volumes** : déclarez dans `meta.conf` (ex : `volumes=data:/data`)
- **Logs** : toutes les sorties sont dans `logs/dockan.log`
- **Isolation** : firejail, systemd-nspawn, chroot (auto-détection)

---

## Roadmap
- [x] Génération du projet
- [x] CLI de base (Go)
- [x] Runtime d’isolation
- [x] Gestion des images Dockan
- [x] Documentation et exemples

---

## Contribution
Forkez, proposez vos idées, partagez vos images Dockan !

---

## Licence
AGPL-3.0 ou ultérieure

## Licence
AGPL-3.0 ou ultérieure
# Dockan
# Dockan
