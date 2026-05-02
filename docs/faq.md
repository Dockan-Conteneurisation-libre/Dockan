# Dockan FAQ

## Does Dockan replace Docker?

Dockan aims to be a local and simple Docker alternative for Linux.

It already covers many important workflows: build, run, logs, exec, volumes, networks, simple compose, and systemd services.

## Does Dockan need a daemon?

No. Dockan does not run a permanent daemon like `dockerd`.

Each command does its work directly.

## Can Dockan run without root?

Yes for simple workflows.

Some advanced features require `sudo`, especially:

- bridge/NAT
- network namespaces and veth
- some mounts
- system-wide installation in `/usr/local/bin`

## Does Dockan download from Docker Hub?

No.

Dockan stays local. To use an Alpine, Ubuntu, or other base, import it from a local rootfs:

```bash
dockan base import alpine:local ./alpine-rootfs
```

## How do dependencies work?

Dockan can explicitly call the package manager of the local machine:

```bash
dockan deps check
sudo dockan deps install -y curl git
```

Supported managers:

- `apt`
- `dnf`
- `apk`
- `pacman`
- `zypper`

Dockan does not install anything secretly.

## Can I use a Dockerfile?

Yes, for simple Dockerfiles.

Dockan understands common instructions: `FROM`, `COPY`, `RUN`, `CMD`, `ENTRYPOINT`, `ENV`, `WORKDIR`, `EXPOSE`, `VOLUME`, `LABEL`, `ARG`, `USER`, `SHELL`, `STOPSIGNAL`, `HEALTHCHECK`.

Use `LABEL` instead of the older Dockan-only `META` instruction.

## Where are images stored?

Images and containers are stored locally in the user’s Dockan store.

List images with:

```bash
dockan images
```

## How do volumes work?

Create a volume:

```bash
dockan volume create data
```

Use the volume:

```bash
dockan run -d --name app -v data:/data app:latest
```

Mount a local folder:

```bash
dockan run -d --name app -v ./data:/data app:latest
```

## Is bridge/NAT networking available?

Yes, with `sudo`.

Example:

```bash
dockan network create appnet --driver bridge --subnet 10.89.0.0/24 --gateway 10.89.0.1/24 --bridge dockan0
sudo dockan network enable appnet
sudo dockan run -d --name web --network appnet -p 8080:80 web:latest
```

## Can Dockan run GUI apps?

Yes for simple GUI apps:

```bash
dockan run --gui app:latest
```

Very complex GUI apps may need manual setup depending on Wayland, X11, audio, GPU, and permissions.

## Can Dockan run on a VPS or Raspberry Pi?

Yes, if the machine runs Linux and has the required tools.

Release packages target Linux `amd64`, `arm64`, and `armv7`.

## How do I publish a clean version?

Create a tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The GitHub Release workflow builds packages and checksums.
