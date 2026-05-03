# Production Guide

This guide is the practical checklist for running Dockan on servers.

Dockan can run server workloads, but production use should be validated on the exact Linux distribution, kernel, isolation mode, network mode, and application profile you plan to use.

## App Updates

Use immutable app versions. Avoid deploying everything as `latest` on servers.

Recommended flow:

```bash
dockan build -t myapp:v1.2.0 .
dockan tag myapp:v1.2.0 myapp:stable
dockan compose redeploy -f /srv/myapp/dockan.yml
```

`dockan compose redeploy` stops the project, removes the old containers, rebuilds services when `build:` is present, and starts the project again.

For systemd services:

```bash
sudo systemctl restart dockan-myapp.service
```

## Rollback

Keep the previous app tag available before updating:

```bash
dockan tag myapp:stable myapp:previous
dockan build -t myapp:v1.3.0 .
dockan tag myapp:v1.3.0 myapp:stable
dockan compose redeploy -f /srv/myapp/dockan.yml
```

If the update fails, point the compose file back to `myapp:previous` or retag it:

```bash
dockan tag myapp:previous myapp:stable
dockan compose redeploy -f /srv/myapp/dockan.yml
```

Keep application data in volumes so rollback does not delete state:

```yaml
services:
  web:
    image: myapp:stable
    volumes:
      - data:/data
```

Back up important volumes before a risky update:

```bash
dockan volume backup data /srv/backups/myapp-data-v1.2.0.tar.gz
```

Restore into a new empty volume when needed:

```bash
dockan volume restore data-restored /srv/backups/myapp-data-v1.2.0.tar.gz
```

## Healthchecks

Put healthchecks in `Dockanfile` or `dockan.yml`:

```yaml
services:
  web:
    image: myapp:stable
    healthcheck: CMD-SHELL curl -f http://127.0.0.1:8080/
```

Check one container:

```bash
dockan health myapp-web
```

Check every service in a compose project:

```bash
dockan compose health -f /srv/myapp/dockan.yml
```

## App And Database Projects

Dockan can run app plus database projects when the database is available as a local Dockan image or a local runtime base.

Use compose with:

- `depends_on` to start the database before the app
- `aliases` so the app can refer to the database as `db`
- `env` for standard names such as `DB_NAME`, `DB_USER`, and `DB_PASSWORD`
- volumes for database state
- healthchecks for app and DB readiness

Example:

```yaml
name: myapp
services:
  web:
    image: myapp:stable
    depends_on:
      - db
    env:
      - DB_HOST=db
      - DB_NAME=myapp
      - DB_USER=myapp
      - DB_PASSWORD=change-me
    network: appnet
    aliases:
      - web
    healthcheck: CMD-SHELL curl -f http://127.0.0.1:8080/
  db:
    image: db:stable
    volumes:
      - db-data:/var/lib/db
    env:
      - DB_NAME=myapp
      - DB_USER=myapp
      - DB_PASSWORD=change-me
    network: appnet
    aliases:
      - db
    healthcheck: CMD-SHELL test -d /var/lib/db
networks:
  - appnet
```

Database init scripts are not universal yet. For now, put init logic inside the DB image, a `hooks/prestart` script, or an app-specific setup command.

## Real Load With Multiple Containers

Before trusting a server, test more than one container:

```bash
dockan compose up -f /srv/myapp/dockan.yml
dockan ps -a
dockan logs myapp-web
dockan logs myapp-db
```

Enable native restart after reboot without adding a Dockan daemon:

```bash
sudo dockan compose autostart -f /srv/myapp/dockan.yml --name myapp
```

For HTTP apps, run an external load tool from another shell or another machine:

```bash
wrk -t4 -c64 -d60s http://127.0.0.1:8080/
```

Or:

```bash
ab -n 5000 -c 50 http://127.0.0.1:8080/
```

Check:

- CPU and RAM usage
- `dockan compose health`
- logs under load
- restart behavior
- port publishing
- volume writes
- cleanup after `dockan compose down`

## Security And Isolation

Run:

```bash
dockan doctor
```

Review the selected isolation mode:

| Mode | Typical use | Notes |
| --- | --- | --- |
| `firejail` | rootless app isolation | Good default when installed. |
| `bubblewrap` | rootless sandboxing | Useful on many desktop/server distros. |
| `systemd-nspawn` | stronger root-based container style | Requires root and a suitable rootfs. |
| `chroot` | basic rootfs isolation | Requires root, weaker than full container isolation. |
| `none` | testing only | Not recommended for untrusted apps. |

For production, prefer:

```bash
dockan run --isolation=firejail myapp:stable
```

or:

```bash
sudo dockan run --isolation=systemd-nspawn myapp:stable
```

Do not run untrusted apps with:

```bash
dockan run --no-isolation myapp:stable
```

## Bridge/NAT Across Linux Distributions

Bridge/NAT requires root and host networking tools:

```bash
sudo dockan deps install -y iproute2 iptables
```

The package names can vary by distribution, so validate with:

```bash
dockan doctor
```

Create and enable a test bridge:

```bash
dockan network create prodnet --driver bridge --subnet 10.89.0.0/24 --gateway 10.89.0.1/24 --bridge dockan0
sudo dockan network enable prodnet
```

Run a container with a published port:

```bash
sudo dockan run -d --name web --network prodnet -p 8080:80 myapp:stable
curl http://127.0.0.1:8080/
```

Check hosts and IPs:

```bash
dockan network hosts prodnet
```

Disable the bridge:

```bash
sudo dockan network disable prodnet
```

Recommended distro validation matrix:

| Distribution | Install | Bridge/NAT | Port publish | Compose | Service |
| --- | --- | --- | --- | --- | --- |
| Ubuntu LTS | required | required | required | required | required |
| Debian stable | required | required | required | required | required |
| Fedora | required | required | required | required | required |
| Alpine | optional | required if targeted | required if targeted | required if targeted | OpenRC/systemd differs |
| Arch | optional | required if targeted | required if targeted | required if targeted | optional |

## Minimum Server Acceptance Checklist

Before calling a Dockan deployment production-ready on a server, verify:

- `dockan doctor` is clean enough for the intended isolation/network mode
- app starts after reboot
- `dockan compose redeploy` updates the app
- rollback to the previous tag works
- volume backup and restore works for important data
- `dockan compose health` passes
- `dockan push` and `dockan pull` work with the chosen local registry folder
- logs are readable
- volumes preserve data after redeploy
- multiple services can run together
- bridge/NAT works on the target distribution
- published ports are reachable
- load test passes for the expected traffic
- service restart behavior is understood

You can run the local acceptance smoke test:

```bash
make build
scripts/server-acceptance.sh
```

Optional bridge/NAT check:

```bash
sudo DOCKAN_ACCEPT_BRIDGE=1 DOCKAN_BIN="$PWD/dockan" scripts/server-acceptance.sh
```

## Practical Production Statement

Use Dockan for local-first Linux deployments, internal services, labs, self-hosting, and controlled server workloads.

For critical workloads, validate the exact server environment before relying on it.
