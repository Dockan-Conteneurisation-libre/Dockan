# Ancienne version d'exemple Dockan

Ce dossier est garde uniquement comme reference historique.

Il montre l'ancien format manuel d'image Dockan avec `meta.conf`, `build.sh`,
`start.sh` et `rootfs/`.

Pour les nouveaux projets, utilisez plutot:

```bash
dockan new python mon-app
dockan build -t mon-app:latest .
dockan run -d --name mon-app -p 8080:8000 mon-app:latest
```

Ou utilisez un fichier `dockan.yml` avec:

```bash
dockan compose up
```

Le format recommande aujourd'hui est:

- `Dockanfile`
- `dockan.yml`
- `examples/`
- `dockan new`
- `dockan compose`

## Ancien format

meta.conf
---------
name=WebApp
port=8080
requires=bash,python3

build.sh
--------
#!/bin/bash
echo "(build.sh) Installation des dépendances..."
# Ex: cp -r src/* rootfs/

start.sh
--------
#!/bin/bash
echo "(start.sh) Lancement de l'app Python..."
cd "$(dirname "$0")/rootfs" && python3 app.py

rootfs/
------
# Placez ici vos fichiers applicatifs (ex: app.py, static/)
