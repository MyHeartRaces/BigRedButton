<p align="center">
  <img src="packaging/assets/big-red-button-wordmark.svg" alt="Big Red Button" width="720">
</p>

# Big Red Button

Big Red Button is a desktop VPN launcher and CLI for a single local profile.

The current build is intended for early Linux testing. It provides a CLI,
desktop GUI, runtime state, Linux route handling, tunnel helper supervision,
and WireGuard setup/rollback primitives. The first supported transport path is
WGWS.

## Status

Alpha. The guarded Linux lifecycle commands can change system routes, create
WireGuard interfaces, start the tunnel helper, and clean up runtime state. Test
in a VM or disposable Arch Linux environment first.

Implemented:

- local VPN profile parser and validator
- secret-redacted profile summary
- connect/disconnect planner
- runtime status snapshots
- Linux route exclusions for the tunnel gateway
- tunnel helper command builder and process executor
- WireGuard `wg setconf` renderer and Linux executor
- composite Linux lifecycle executor with rollback tests
- guarded Linux connect/disconnect CLI commands
- Linux isolated app tunnel planner, dry-run, guarded apply, stop and cleanup
- desktop GUI launcher with system and Linux isolated app controls
- Arch Linux application launcher package
- macOS `.pkg` installer with an app bundle
- Windows amd64 installer with Start Menu/Desktop shortcuts

Not implemented yet:

- privileged daemon / IPC boundary
- DNS adapter
- automatic isolated session crash recovery
- Windows, macOS, or mobile ports

## Requirements

Build-time:

- Go 1.24 or newer
- `make`

Runtime on Linux:

- `iproute2`
- `wireguard-tools`
- `nftables` for isolated app tunnel fail-closed rules
- `setpriv` from util-linux for launching isolated apps as the desktop user
- `wstunnel` in `PATH`, or pass `-wstunnel-binary /path/to/wstunnel`
- root privileges or equivalent capabilities for real connect/disconnect

On Arch Linux, `iproute2` and `wireguard-tools` are official packages. The
`wstunnel` helper may need to be installed separately if it is not available in
your configured repositories.

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
browser. It can save a local VPN profile, show redacted profile details, show
runtime status, and on Linux run guarded connect/disconnect commands through
the CLI.

On Linux the GUI uses `pkexec` when available, so desktop environments can show
a graphical privilege prompt. The Linux package installs a polkit action for
`/usr/bin/big-red-button` so the prompt uses the application name and icon.
On macOS and Windows the GUI starts normally, but real connect/disconnect
remains unavailable until those platform adapters are implemented.

## GitHub Releases

Tagged releases are published to GitHub Releases:

<https://github.com/MyHeartRaces/BigRedButton/releases>

Release assets include Windows, macOS arm64, Arch Linux package, and
`SHA256SUMS` files. Maintainers publish a release by pushing a `v*` tag, for
example:

```bash
git tag -a v0.2.1 -m "v0.2.1"
git push origin v0.2.1
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
- `/usr/share/polkit-1/actions/com.myheartraces.bigredbutton.policy`
- `/usr/share/licenses/big-red-button/LICENSE`
- `/usr/share/doc/big-red-button/README.md`

The PKGBUILD is in `packaging/arch/PKGBUILD`.

## Quick Smoke Test

```bash
big-red-button validate-profile testdata/profiles/valid-wgws.json
big-red-button plan-connect \
  -endpoint-ip 203.0.113.10 \
  testdata/profiles/valid-wgws.json
big-red-button status
```

Dry-run Linux route planning:

```bash
big-red-button linux-dry-run-connect \
  -endpoint-ip 203.0.113.10 \
  -default-gateway 192.0.2.1 \
  -default-interface eth0 \
  testdata/profiles/valid-wgws.json
```

## Real Linux Connect

These commands change networking state. Run them only on the test machine.

```bash
sudo big-red-button linux-connect \
  -yes \
  -endpoint-ip <resolved-tunnel-gateway-ip> \
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

## Linux Isolated App Tunnel

This mode launches one selected process inside a Linux network namespace. It
does not change host default routes or host DNS. The namespace gets its own
`veth`, WireGuard interface, DNS file and namespace-only `nft` fail-closed
rules.

Plan only:

```bash
big-red-button plan-isolated-app \
  -session-id 123e4567-e89b-12d3-a456-426614174000 \
  /path/to/profile.json -- /usr/bin/curl https://example.com
```

Dry-run:

```bash
big-red-button linux-dry-run-isolated-app \
  -session-id 123e4567-e89b-12d3-a456-426614174000 \
  /path/to/profile.json -- /usr/bin/curl https://example.com
```

Real run:

```bash
sudo big-red-button linux-isolated-app \
  -yes \
  -session-id 123e4567-e89b-12d3-a456-426614174000 \
  -wstunnel-binary /usr/bin/wstunnel \
  /path/to/profile.json -- /usr/bin/curl https://example.com
```

When launched through `sudo` or `pkexec`, the CLI tries to infer the desktop
user from `SUDO_UID`/`SUDO_GID` or `PKEXEC_UID` and launches the selected app
with `setpriv` inside the namespace. You can override this explicitly with
`-app-uid <uid> -app-gid <gid>`.

Desktop display environment can be forwarded with repeatable
`-app-env KEY=value`. Only display/session keys are accepted: `DISPLAY`,
`WAYLAND_DISPLAY`, `XAUTHORITY`, `XDG_RUNTIME_DIR`,
`DBUS_SESSION_BUS_ADDRESS`, `PULSE_SERVER`, and `PIPEWIRE_RUNTIME_DIR`. The GUI
forwards those keys automatically when they are present.

Stop and cleanup:

```bash
sudo big-red-button linux-stop-isolated-app \
  -yes \
  -session-id 123e4567-e89b-12d3-a456-426614174000
```

If a session is already dirty and runtime state is missing or stale, run the
best-effort cleanup command for the same session UUID:

```bash
sudo big-red-button linux-cleanup-isolated-app \
  -yes \
  -session-id 123e4567-e89b-12d3-a456-426614174000
```

Status:

```bash
big-red-button isolated-status \
  -session-id 123e4567-e89b-12d3-a456-426614174000
```

List all known isolated runtime sessions:

```bash
big-red-button isolated-sessions
```

On Linux, isolated status marks a session `Dirty` if saved app or WSTunnel PIDs
are no longer present in `/proc`; use stop first, then cleanup if normal stop
cannot finish.

Isolated session state is stored under
`/run/big-red-button/isolated/<session-id>/state.json`.

## Profile

The expected profile is a normalized JSON VPN profile for the currently
supported WGWS adapter. See `testdata/profiles/valid-wgws.json` for the current
schema.

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
