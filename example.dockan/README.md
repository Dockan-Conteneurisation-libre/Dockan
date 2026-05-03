# Legacy Dockan Example

This directory is kept only as a historical reference.

It shows the old manual Dockan image format with `meta.conf`, `build.sh`,
`start.sh`, and `rootfs/`.

For new projects, use:

```bash
dockan new python my-app
dockan build -t my-app:latest .
dockan run -d --name my-app -p 8080:8000 my-app:latest
```

Or use a `dockan.yml` file with:

```bash
dockan compose up
```

The recommended format today is:

- `Dockanfile`
- `dockan.yml`
- `examples/`
- `dockan new`
- `dockan compose`

## Old Format

### `meta.conf`

```ini
name=WebApp
port=8080
requires=bash,python3
```

### `build.sh`

```bash
#!/bin/bash
echo "(build.sh) Installing dependencies..."
# Ex: cp -r src/* rootfs/
```

### `start.sh`

```bash
#!/bin/bash
echo "(start.sh) Starting the Python app..."
cd "$(dirname "$0")/rootfs" && python3 app.py
```

### `rootfs/`

```text
Put application files here, for example app.py or static assets.
```
