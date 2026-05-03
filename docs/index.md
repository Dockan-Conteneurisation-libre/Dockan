# Dockan

Dockan is a local Docker alternative for Linux.

Goal: run containerized applications without a daemon, without a required cloud service, with simple commands and readable files.

Dockan is made to:

- build images from a `Dockanfile` or a simple local `Dockerfile`
- generate starter apps for PHP, Node.js, Python, Go, Rust, Java, Ruby, shell, and static binaries
- run apps in foreground or in the background
- manage logs, volumes, networks, ports, and systemd services
- use a `dockan.yml` file to run multiple services
- manage Dockan from the optional Dockan Panel web UI
- stay local: no forced registry, no permanent daemon

## Dockan vs Docker

Dockan is a real Docker alternative for local Linux workflows. It is built for people who want Docker-like app running without a permanent daemon, without a required cloud registry, and with files that remain easy to inspect.

Choose Dockan for:

- local-first self-hosting
- simple app packaging
- folder-based image sharing
- systemd services
- labs, education, internal tools, and small servers
- readable builds and explicit dependencies

Choose Docker for:

- full Docker Hub compatibility
- advanced OCI layers and cache behavior
- full Dockerfile compatibility
- mature dynamic internal DNS
- Kubernetes-oriented production fleets
- the largest third-party ecosystem

Dockan is simpler and more local. Docker is broader and more mature.

## Install

User install, without sudo:

```bash
curl -fsSL https://raw.githubusercontent.com/Dockan-Conteneurisation-libre/Dockan/main/scripts/install.sh | sh
```

If `~/.local/bin` is not in your `PATH`, add it:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

System-wide install:

```bash
curl -fsSL https://raw.githubusercontent.com/Dockan-Conteneurisation-libre/Dockan/main/scripts/install.sh | sudo sh
```

From a local checkout:

```bash
sudo INSTALL_SOURCE=source sh scripts/install.sh
```

Check the machine:

```bash
dockan version
dockan doctor
dockan ps -a
```

Update later:

```bash
dockan update
```

Update to a specific release:

```bash
dockan update --version v0.1.1
```

Update a system-wide installation:

```bash
dockan update --system
```

## Dockan Panel

Dockan Panel is the optional browser UI for Dockan. It can manage containers,
images, volumes, stacks, backups, and live terminals.

When the panel is started with Dockan Compose, its persistent data is stored in
the `dockan-panel-data` volume. The admin auth database is:

```text
/app/storage/auth-users.json
```

On a normal user install, that volume is on the host under:

```text
~/.local/share/dockan/volumes/dockan-panel-data
```

That volume contains panel users, password hashes, 2FA/TOTP secrets, passkey
public keys, stacks, and panel backups. Removing the volume removes that panel
state.

```bash
cd /path/to/Dockan-Panel
dockan compose up
```

Open `http://127.0.0.1:9090`, then create the first admin account. There is no
default password and no default token.

Passkeys work on `localhost`, `127.0.0.1`, or HTTPS. Browsers usually block
passkeys on plain HTTP LAN addresses.

## First Test

```bash
dockan build -t hello:latest examples/hello
dockan run hello:latest
dockan images
```

## Create An App

Start from a language template:

```bash
dockan new php my-php-app
dockan new node my-node-app
dockan new python my-python-app
dockan new binary my-binary-app
```

Create a `Dockanfile`:

```dockerfile
FROM scratch
LABEL org.opencontainers.image.title=MyApp
COPY app.sh /app.sh
RUN chmod +x /app.sh
CMD ./app.sh
```

Create `app.sh`:

```bash
#!/usr/bin/env sh
echo "Hello from Dockan"
```

Build and run:

```bash
dockan build -t myapp:latest .
dockan run myapp:latest
```

Dockan can also read a simple local `Dockerfile` when no `Dockanfile` exists.

## Run Any Language

Dockan can run many Linux languages when the runtime exists in the local base or the app ships as a Linux binary:

```dockerfile
FROM php:8.3
WORKDIR /app
COPY . /app
EXPOSE 8000
CMD php -S 0.0.0.0:8000 -t public
```

No Docker Hub is required here: if `php:8.3` is not imported as a Dockan base, Dockan uses the local `php` command.

Install the runtime when it is missing:

```bash
sudo dockan deps runtime php:8.3 -y
```

Guide: [Run Any Language](./languages.html)

## Background Containers

```bash
dockan run -d --name myapp myapp:latest
dockan ps -a
dockan logs myapp
dockan exec myapp sh
dockan stop myapp
dockan rm myapp
```

## Volumes

Create a named volume:

```bash
dockan volume create data
```

Run an app with a volume:

```bash
dockan run -d --name web -v data:/data web:latest
```

Mount a local folder:

```bash
dockan run -d --name web -v ./data:/data web:latest
```

Back up and restore volumes:

```bash
dockan volume backup data data-backup.tar.gz
dockan volume restore data-restored data-backup.tar.gz
```

## Network

Simple local network:

```bash
dockan network create appnet
dockan run -d --name web --network appnet --alias web.local web:latest
dockan network refresh appnet
dockan network hosts appnet
dockan network doctor
```

Bridge/NAT with published ports requires `sudo`:

```bash
dockan network create appnet --driver bridge --subnet 10.89.0.0/24 --gateway 10.89.0.1/24 --bridge dockan0
sudo dockan network enable appnet
sudo dockan run -d --name web --network appnet -p 8080:80 web:latest
```

Disable the bridge:

```bash
sudo dockan network disable appnet
```

## Simple Compose

Create `dockan.yml`:

```yaml
name: myapp
services:
  web:
    build: .
    image: web:latest
    ports:
      - 8080:8080
    env:
      - PORT=8080
    volumes:
      - data:/data
    network: appnet
    aliases:
      - web.local
    restart: always
    healthcheck: CMD-SHELL curl -f http://127.0.0.1:8080/
    memory: 512m
    cpus: 1.5
  db:
    image: db:latest
    volumes:
      - db-data:/var/lib/db
    env:
      - DB_NAME=myapp
      - DB_USER=myapp
      - DB_PASSWORD=change-me
    aliases:
      - db
    network: appnet
    healthcheck: CMD-SHELL test -d /var/lib/db
networks:
  - appnet
```

Run:

```bash
dockan compose up
dockan compose down
dockan compose health
```

For app plus database projects, Dockan supports `depends_on`, network aliases, environment variables, persistent volumes, and healthchecks. Database init scripts and standard user/password behavior still need to be implemented by the database image or an app hook.

## Known App Example: WordPress-Style Stack

Dockan can run a WordPress-style PHP app with a MariaDB/MySQL database when the images are prepared locally:

```yaml
name: wordpress
services:
  web:
    image: wordpress:local
    ports:
      - 8080:8080
    env:
      - DB_HOST=db
      - DB_NAME=wordpress
      - DB_USER=wordpress
      - DB_PASSWORD=change-me
    volumes:
      - wp-data:/var/www/html
    network: wpnet
    aliases:
      - web
    depends_on:
      - db
    restart: always
    healthcheck: CMD-SHELL curl -f http://127.0.0.1:8080/
  db:
    image: mariadb:local
    env:
      - DB_NAME=wordpress
      - DB_USER=wordpress
      - DB_PASSWORD=change-me
      - DB_ROOT_PASSWORD=change-root
    volumes:
      - db-data:/var/lib/mysql
    network: wpnet
    aliases:
      - db
    restart: always
    healthcheck: CMD-SHELL test -d /var/lib/mysql
networks:
  - wpnet
```

```bash
dockan compose up
dockan compose health
dockan volume backup db-data wordpress-db-backup.tar.gz
```

This keeps the app local. Dockan does not pull `wordpress` or `mariadb` from Docker Hub automatically; those images must be created, imported, or shared as local Dockan images.

## Local Registry

Use a normal folder as a local registry:

```bash
dockan push myapp:latest /srv/dockan-registry
dockan registry ls /srv/dockan-registry
dockan pull myapp:latest /srv/dockan-registry
```

The registry folder contains archives, checksums, and an index. It does not need Docker Hub or a daemon.

## Install As A Service

System service with sudo:

```bash
sudo dockan service install -f /srv/myapp/dockan.yml --name myapp
sudo systemctl daemon-reload
sudo systemctl enable --now dockan-myapp.service
```

User service:

```bash
dockan service install --user -f ~/myapp/dockan.yml --name myapp
systemctl --user daemon-reload
systemctl --user enable --now dockan-myapp.service
```

## Developer Guide

A developer usually shares:

```text
myapp/
  Dockanfile
  dockan.yml
  README.md
  app.sh
  src/
```

The admin installs Dockan, fetches the app, builds locally, and runs locally:

```bash
curl -fsSL https://raw.githubusercontent.com/Dockan-Conteneurisation-libre/Dockan/main/scripts/install.sh | sh
tar -xzf myapp-dockan-v1.tar.gz
cd myapp
dockan build -t myapp:v1 .
dockan compose up
```

Full guide: [Developer Guide](./developer.html)

## Production

Dockan is usable as a simple local container tool: build, run, logs, exec, volumes, networks, compose, and services are available.

For a clean production install, publish a GitHub Release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The release workflow builds Linux packages and publishes checksums.

## What Already Works

- `curl | sh` installer
- Linux `.tar.gz` packages and `.deb` when possible
- local images
- `Dockanfile` and simple `Dockerfile`
- `.dockerignore`
- simple multi-stage builds with `COPY --from`
- local bases imported from a folder or archive
- detached containers
- logs, `exec`, `stop`, `rm`, `inspect`
- healthchecks with `dockan health` and `dockan compose health`
- environment variables
- named volumes, local folders, backup, and restore
- simple networks and bridge/NAT with sudo
- port publishing with `-p`
- `dockan compose`
- systemd services
- simple GUI apps with `--gui`
- explicit dependency installation through `apt`, `dnf`, `apk`, `pacman`, or `zypper`

## More

- [Dockan Format](./format.html)
- [Run Any Language](./languages.html)
- [Developer Guide](./developer.html)
- [Production Guide](./production.html)
- [Python Example](./exemples/python.html)
- [Export and Import](./exemples/export.html)
- [FAQ](./faq.html)

License: `AGPL-3.0-or-later`. See the repository `LICENSE` file for the full license text.
