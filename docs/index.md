# Dockan : Conteneurisation libre, sans daemon, sans cloud

Bienvenue sur la documentation officielle de Dockan, l’alternative anarchiste à Docker !

## Présentation
Dockan est un système de conteneurisation minimaliste, sans daemon, sans registre centralisé, conçu pour l’auto-hébergement, l’expérimentation, l’éducation et la liberté numérique.

- **Isolation** : firejail, systemd-nspawn, chroot (auto-détection)
- **Format ouvert** : dossier `.dockan/` ou archive `.tar.gz`
- **Pas de cloud, pas de DockerHub, pas de capitalisme logiciel**
- **Partage facile** : un dossier = une image, export/import natif

## Installation rapide
```bash
git clone <repo> dockan
cd dockan
go build -o dockan ./cmd/dockan.go
sudo mv dockan /usr/local/bin/
dockan help
```

## Créer et lancer une app
```bash
dockan init monapp.dockan
# Placez votre code dans monapp.dockan/rootfs/
# Adaptez start.sh et meta.conf

dockan run monapp.dockan
```

## Fonctionnalités principales
- Build, run, export, import, hooks, volumes, logs
- Pas de daemon, pas de root obligatoire (avec firejail)
- Images portables, reproductibles, auditables

## Exemples
- [Créer une image Python](./exemples/python.md)
- [Exporter/Importer une image](./exemples/export.md)

## Pour aller plus loin
- [Format d’une image Dockan](./format.md)
- [FAQ](./faq.md)
- [Roadmap](../README.md#roadmap)

---

Pour contribuer, forkez et proposez vos idées !

Licence : AGPL-3.0+
