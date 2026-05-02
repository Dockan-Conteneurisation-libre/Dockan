# Export And Import

Dockan stays local. To share an app, you can share the project files, tag images, or import local bases from files.

## Built Images

Build an image:

```bash
dockan build -t myapp:latest .
```

List images:

```bash
dockan images
```

Add another tag:

```bash
dockan tag myapp:latest myapp:v1
```

Remove an image:

```bash
dockan rmi myapp:v1
```

## Local Bases

Import a base from a rootfs folder:

```bash
dockan base import alpine:local ./alpine-rootfs
```

Import from an archive:

```bash
dockan base import ubuntu:local ./ubuntu-rootfs.tar.gz
```

Use the base:

```dockerfile
FROM alpine:local
COPY app.sh /app.sh
CMD ./app.sh
```

## Legacy .dockan Format

The old folder format can still be exported and imported:

```bash
dockan export myapp.dockan myapp.tar.gz
dockan import myapp.tar.gz myapp.dockan
```

For new projects, prefer `dockan build -t name:tag .`.
