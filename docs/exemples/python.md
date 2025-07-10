# Exemple : Image Python avec Dockan

## Structure
```
monapp.dockan/
  meta.conf
  build.sh
  start.sh
  rootfs/
    app.py
```

## meta.conf
```properties
name=MonApp
port=8080
requires=python3
```

## start.sh
```bash
#!/bin/bash
cd "$(dirname "$0")/rootfs"
python3 app.py
```

## Commandes
```bash
dockan init monapp.dockan
# Placez app.py dans rootfs/
dockan run monapp.dockan
```
