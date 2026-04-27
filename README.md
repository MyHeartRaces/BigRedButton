# Big Red Button

Big Red Button is a headless Linux client for a single V7 profile:
WireGuard over WSTunnel.

The current build is intended for early Linux testing. It provides a CLI,
runtime state, Linux route handling, WSTunnel process supervision, and
WireGuard setup/rollback primitives. The desktop UI is not implemented yet.

## Status

Alpha. The guarded Linux lifecycle commands can change system routes, create
WireGuard interfaces, start WSTunnel, and clean up runtime state. Test in a VM
or disposable Arch Linux environment first.

Implemented:

- V7 profile parser and validator
- secret-redacted profile summary
- connect/disconnect planner
- runtime status snapshots
- Linux endpoint route exclusions
- WSTunnel command builder and process executor
- WireGuard `wg setconf` renderer and Linux executor
- composite Linux lifecycle executor with rollback tests
- guarded Linux connect/disconnect CLI commands
- Arch Linux package build files

Not implemented yet:

- desktop UI
- privileged daemon / IPC boundary
- DNS adapter
- kill switch
- Windows, macOS, or mobile ports

## Requirements

Build-time:

- Go 1.24 or newer
- `make`

Runtime on Linux:

- `iproute2`
- `wireguard-tools`
- `wstunnel` in `PATH`, or pass `-wstunnel-binary /path/to/wstunnel`
- root privileges or equivalent capabilities for real connect/disconnect

On Arch Linux, `iproute2` and `wireguard-tools` are official packages. WSTunnel
may need to be installed separately if it is not available in your configured
repositories.

## Build

```bash
make test
make build
./build/big-red-button help
```

The binary is written to `build/big-red-button`.

## Arch Linux Package

From a clean checkout on Arch Linux:

```bash
./scripts/build-arch-package.sh
sudo pacman -U dist/arch/makepkg/big-red-button-*.pkg.tar.zst
```

The package installs:

- `/usr/bin/big-red-button`
- `/usr/share/licenses/big-red-button/LICENSE`
- `/usr/share/doc/big-red-button/README.md`

The PKGBUILD is in `packaging/arch/PKGBUILD`.

## Quick Smoke Test

```bash
big-red-button validate-profile testdata/profiles/valid-v7.json
big-red-button plan-connect \
  -endpoint-ip 203.0.113.10 \
  testdata/profiles/valid-v7.json
big-red-button status
```

Dry-run Linux route planning:

```bash
big-red-button linux-dry-run-connect \
  -endpoint-ip 203.0.113.10 \
  -default-gateway 192.0.2.1 \
  -default-interface eth0 \
  testdata/profiles/valid-v7.json
```

## Real Linux Connect

These commands change networking state. Run them only on the test machine.

```bash
sudo big-red-button linux-connect \
  -yes \
  -endpoint-ip <resolved-wstunnel-server-ip> \
  -wstunnel-binary /usr/bin/wstunnel \
  /path/to/profile.json
```

Disconnect:

```bash
sudo big-red-button linux-disconnect -yes /path/to/profile.json
```

Status:

```bash
big-red-button status
```

By default, runtime state is stored in `/run/big-red-button/state.json`.

## Profile

The expected profile is a normalized V7 WireGuard-over-WSTunnel JSON profile.
See `testdata/profiles/valid-v7.json` for the current schema.

The planner and runtime status never print private keys. The WireGuard executor
does write a temporary `wg setconf` file with private key material while
configuring the interface, then removes it after `wg setconf` returns.

## Development

```bash
go vet ./...
go test ./...
go run ./cmd/big-red-button help
```

## License

MIT. See `LICENSE`.
