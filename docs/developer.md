# Developer Guide

This page explains how developers can package and share Dockan apps so admins can run them locally on Linux machines.

Dockan separates two jobs:

- developers prepare the app with `Dockanfile`, `dockan.yml`, code, and a README
- admins install Dockan on the target machine, build the app locally, and run it locally

## Developer Side

Recommended structure:

```text
myapp/
  Dockanfile
  dockan.yml
  README.md
  app.sh
  src/
```

Example `Dockanfile`:

```dockerfile
FROM scratch
LABEL org.opencontainers.image.title=MyApp
COPY app.sh /app.sh
RUN chmod +x /app.sh
CMD ./app.sh
```

Example `dockan.yml`:

```yaml
name: myapp
services:
  web:
    build: .
    image: myapp:latest
    ports:
      - 8080:8080
    volumes:
      - data:/data
    restart: always
```

The developer can share the app through GitHub, GitLab, a `.tar.gz` archive, an internal server, or offline media.

## Admin Side

Install Dockan:

```bash
curl -fsSL https://raw.githubusercontent.com/Dockan-Conteneurisation-libre/Dockan/main/scripts/install.sh | sh
```

Fetch the app:

```bash
tar -xzf myapp-dockan-v1.tar.gz
cd myapp
```

Build and run:

```bash
dockan build -t myapp:v1 .
dockan compose up
```

Check logs:

```bash
dockan ps -a
dockan logs myapp-web
```

## Start On Boot

Enable native auto-start through systemd:

```bash
sudo dockan compose autostart -f /srv/myapp/dockan.yml --name myapp
```

User service without sudo:

```bash
dockan compose autostart --user -f ~/myapp/dockan.yml --name myapp
```

Disable auto-start:

```bash
sudo dockan compose no-autostart -f /srv/myapp/dockan.yml --name myapp
```

## Local Base Images

If the app needs a full Ubuntu, Alpine, or Python base, the developer can provide a rootfs archive:

```bash
dockan base import ubuntu:local ./ubuntu-rootfs.tar.gz
dockan build -t myapp:v1 .
```

Then the `Dockanfile` can use the local base:

```dockerfile
FROM ubuntu:local
COPY app.sh /app.sh
CMD ./app.sh
```

## Network And Ports

Simple network:

```bash
dockan network create appnet
dockan run -d --name web --network appnet myapp:v1
```

Bridge/NAT with port publishing:

```bash
dockan network create appnet --driver bridge --subnet 10.89.0.0/24 --gateway 10.89.0.1/24 --bridge dockan0
sudo dockan network enable appnet
sudo dockan run -d --name web --network appnet -p 8080:80 myapp:v1
```

## Summary

To share a Dockan app:

```text
developer -> Dockanfile + dockan.yml + code + README
admin -> install Dockan, local build, local run, systemd service
```

Dockan itself is distributed through GitHub Releases. Dockan apps are normal application projects that can be shared freely.
