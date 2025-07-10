# Exemple d'image Dockan avancée

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
