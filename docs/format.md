# Format d’une image Dockan

Une image Dockan est un dossier (ou une archive) avec la structure suivante :

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

- **meta.conf** : nom, ports, dépendances, volumes, etc.
- **build.sh** : instructions de build (optionnel)
- **start.sh** : point d’entrée du conteneur
- **rootfs/** : tout le système de fichiers de l’app
- **hooks/** : scripts exécutés avant/après (prestart, poststop)
- **volumes/** : dossiers persistants
- **logs/** : logs d’exécution
