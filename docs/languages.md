# Run Any Language

Dockan is not tied to one language. It can run any Linux app when one of these is true:

- the app is a Linux static binary and can run from `FROM scratch`
- the runtime exists on the machine in the selected isolation mode
- a local Dockan base contains the runtime, for example `python:local`, `php:local`, or `node:local`

The Docker-like rule is simple: the app must have what it needs to start. Dockan can use Docker-style names such as `php:8.3`, `node:20`, `python:3.12`, `openjdk:21`, or `golang:1.22` without Docker Hub by using the local host runtime.

## Start A Project

```bash
dockan new python my-python-app
dockan new node my-node-app
dockan new php my-php-app
dockan new go my-go-app
dockan new rust my-rust-app
dockan new java my-java-app
dockan new ruby my-ruby-app
dockan new binary my-binary-app
```

Then:

```bash
cd my-python-app
dockan build -t my-python-app:latest .
dockan run -d --name my-python-app -p 8080:8000 my-python-app:latest
```

## PHP

```dockerfile
FROM php:8.3
WORKDIR /app
COPY . /app
EXPOSE 8000
CMD php -S 0.0.0.0:8000 -t public
```

## Node.js

```dockerfile
FROM node:20
WORKDIR /app
COPY . /app
EXPOSE 3000
CMD node server.js
```

## Python

```dockerfile
FROM python:3.12
WORKDIR /app
COPY . /app
EXPOSE 8000
CMD python3 app.py
```

## Go Or Rust

For production, the strongest path is often to build a Linux binary first, then use:

```dockerfile
FROM scratch
COPY app /app
EXPOSE 8080
CMD ./app
```

## Local Bases

Dockan stays local. It does not automatically pull `php:8`, `node:20`, or `python:3` from Docker Hub.

If you write `FROM php:8.3` and no `php:8.3` Dockan base exists, Dockan checks the host for `php`. If `php` is installed, the build continues as a host-runtime base. If `php` is missing, Dockan stops with a clear error.

Install a missing runtime explicitly:

```bash
dockan deps runtime php:8.3 --dry-run
sudo dockan deps runtime php:8.3 -y
sudo dockan deps runtime node:20 -y
sudo dockan deps runtime python:3.12 -y
```

Import a local rootfs or archive:

```bash
dockan base runtime php:8.3 --from ./php83-rootfs
dockan base runtime node:20 --from ./node20-rootfs.tar.gz
dockan base import php:local ./php-rootfs
dockan base import node:local ./node-rootfs.tar.gz
dockan base import python:local ./python-rootfs.tar.gz
```

Then use the base in `Dockanfile`.

## Important

Dockan can run many languages, but it cannot magically run a language if the runtime is missing. For PHP you need PHP, for Node you need Node, for Java you need a JRE/JDK, and for Python you need Python.
