<p align="center">
  <img src="packaging/assets/big-red-button-wordmark.svg" alt="Big Red Button" width="720">
</p>

# Big Red Button

Big Red Button is a desktop launcher and CLI for a single V7 profile:
WireGuard over WSTunnel.

The current build is intended for early Linux testing. It provides a CLI,
desktop GUI, runtime state, Linux route handling, WSTunnel process supervision,
and WireGuard setup/rollback primitives.

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
- desktop GUI launcher
- Arch Linux application launcher package
- macOS `.pkg` installer with an app bundle
- Windows amd64 installer with Start Menu/Desktop shortcuts

Not implemented yet:

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
./build/big-red-button-gui -addr 127.0.0.1:0 -no-open
```

The binaries are written to `build/big-red-button` and
`build/big-red-button-gui`.

## Desktop GUI

`big-red-button-gui` starts a local desktop web UI and opens it in the default
browser. It can save a V7 profile, show redacted profile details, show runtime
status, and on Linux run guarded connect/disconnect commands through the CLI.

On Linux the GUI uses `pkexec` when available, so desktop environments can show
a graphical privilege prompt. On macOS and Windows the GUI starts normally, but
real connect/disconnect remains unavailable until those platform adapters are
implemented.

## GitHub Releases

Tagged releases are published to GitHub Releases:

<https://github.com/MyHeartRaces/BigRedButton/releases>

Release assets include Windows, macOS arm64, Arch Linux package, and
`SHA256SUMS` files. Maintainers publish a release by pushing a `v*` tag, for
example:

```bash
git tag -a v0.2.0 -m "v0.2.0"
git push origin v0.2.0
```

## GitHub Actions Builds

The repository includes `.github/workflows/build.yml`.

It builds and uploads artifacts for:

- Windows 11 arm64: `big-red-button_windows_11_arm64.zip`
- Windows amd64 compatible: `big-red-button_windows_amd64.zip`
- Windows amd64 installer: `BigRedButtonSetup-*-windows-amd64.exe`
- macOS arm64 installer: `big-red-button_darwin_arm64.pkg`
- macOS arm64 app bundle ZIP: `big-red-button_darwin_arm64_app.zip`
- Arch Linux package: `big-red-button-*.pkg.tar.*`

The Windows amd64 job runs on GitHub's hosted Windows Server runner because
GitHub does not provide a standard hosted Windows 11 x64 runner. Use a
self-hosted or larger Windows 11 x64 runner if that distinction becomes
important for UI/system-integration testing.

## Arch Linux Package

From a clean checkout on Arch Linux:

```bash
./scripts/build-arch-package.sh
sudo pacman -U dist/arch/makepkg/big-red-button-*.pkg.tar.zst
```

The package installs:

- `/usr/bin/big-red-button`
- `/usr/bin/big-red-button-gui`
- `/usr/share/applications/big-red-button.desktop`
- `/usr/share/icons/hicolor/scalable/apps/big-red-button.svg`
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
