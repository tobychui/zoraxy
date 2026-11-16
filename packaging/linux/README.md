# Linux packages (.deb and .rpm)

Build Debian/Ubuntu and RHEL/Fedora packages of Zoraxy from this source tree.

```sh
make -C packaging/linux packages            # .deb and .rpm for the host architecture
make -C packaging/linux deb ARCH=arm64      # amd64 arm64 arm 386 riscv64
make -C packaging/linux rpm BINARY=/path/to/zoraxy_linux_amd64 ARCH=amd64   # package a prebuilt binary
```

Requirements: Go (unless `BINARY` is set), `dpkg-deb` for `.deb`, `rpmbuild` for `.rpm`.
Packages are written to `packaging/linux/dist/`. `VERSION` defaults to `SYSTEM_VERSION` in `src/def.go`; `RELEASE` defaults to `1`.

The `Linux packages` workflow builds both formats for amd64, arm64 and arm, tests them
(systemd service on Ubuntu; install and run on Oracle Linux 8/9 and Fedora) and attaches
them to each published release.

## What the packages install

| Path | Content |
|---|---|
| `/usr/bin/zoraxy` | static binary |
| `/usr/lib/systemd/system/zoraxy.service` | systemd unit (user `zoraxy`, `CAP_NET_BIND_SERVICE` for ports 80/443) |
| `/etc/default/zoraxy` (deb), `/etc/sysconfig/zoraxy` (rpm) | `ZORAXY_OPTS`: command line options |
| `/var/lib/zoraxy` | data directory (working directory of the service) |

The service is **not started automatically**: on first visit the management interface
(`:8000` by default) asks to create the admin account, so review the options first, e.g.
`ZORAXY_OPTS="-port=127.0.0.1:8000"`, then:

```sh
systemctl enable --now zoraxy
```

The packages are not signed: install them with `apt install ./zoraxy_*.deb` or
`dnf install ./zoraxy-*.rpm` (rpm 6, e.g. Fedora 43, refuses unsigned packages with plain `rpm -i`).

Upgrades restart the service if it is running. Removing the package keeps `/var/lib/zoraxy`
and the `zoraxy` user.
