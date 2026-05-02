# Dockan Format

Dockan supports two simple formats.

## Recommended Format: Dockanfile

The recommended format looks like a Dockerfile, while staying local.

Example:

```dockerfile
FROM scratch
LABEL org.opencontainers.image.title=MyApp
WORKDIR /app
COPY . /app
RUN chmod +x ./start.sh
CMD ./start.sh
```

Build:

```bash
dockan build -t myapp:latest .
```

Run:

```bash
dockan run myapp:latest
```

Dockan reads `Dockanfile` first. If no `Dockanfile` exists, it can read a simple local `Dockerfile`.

## Supported Instructions

- `FROM`
- `COPY`
- `ADD`
- `RUN`
- `CMD`
- `ENTRYPOINT`
- `ENV`
- `WORKDIR`
- `EXPOSE`
- `VOLUME`
- `LABEL`
- `ARG`
- `USER`
- `SHELL`
- `STOPSIGNAL`
- `HEALTHCHECK`

Dockan also supports:

- `.dockerignore`
- simple multi-stage builds with `FROM ... AS ...`
- `COPY --from=...`
- Docker exec-form commands such as `CMD ["node", "server.js"]`
- local bases imported with `dockan base import`

## Local Bases

Import a base:

```bash
dockan base import alpine:local ./alpine-rootfs
```

Or from an archive:

```bash
dockan base import ubuntu:local ./ubuntu-rootfs.tar.gz
```

Use the base:

```dockerfile
FROM alpine:local
COPY app.sh /app.sh
CMD ./app.sh
```

Dockan does not automatically download from Docker Hub. The base must exist locally.

## Legacy Format: .dockan Folder

The older folder format still works for very simple apps:

```text
myapp.dockan/
  meta.conf
  build.sh
  start.sh
  rootfs/
  hooks/
  volumes/
  logs/
```

File roles:

- `meta.conf`: legacy metadata
- `build.sh`: optional build script
- `start.sh`: entrypoint
- `rootfs/`: application files
- `hooks/`: scripts executed before or after some actions
- `volumes/`: local persistent data
- `logs/`: execution logs

For new projects, prefer `Dockanfile`.
