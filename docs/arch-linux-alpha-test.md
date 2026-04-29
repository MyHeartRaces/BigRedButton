# Arch Linux Alpha Test Checklist

Date: 2026-04-29

Status: local alpha testing guide

Use this checklist for the first Linux/Arch validation pass. Prefer a VM or a
machine where temporary network disruption is acceptable.

## 0. Repository State

The latest local implementation may be ahead of GitHub. If the package is built
from GitHub Actions or GitHub Releases, push the local commits first and wait
for the Arch package artifact. If testing from a local checkout on Arch, build
directly from that checkout.

## 1. Install Dependencies

```bash
sudo pacman -Syu
sudo pacman -S --needed git base-devel go
```

The release package declares the runtime dependencies and pacman resolves them
when installing the package. It also bundles a pinned upstream WSTunnel helper
at `/usr/lib/big-red-button/wstunnel`.

## 2. Install Big Red Button

From a downloaded package artifact:

```bash
sudo pacman -U ./big-red-button-*.pkg.tar.zst
```

From a clean Arch checkout:

```bash
git clone https://github.com/MyHeartRaces/BigRedButton.git
cd BigRedButton
PKGVER=0.2.1 ./scripts/build-arch-package.sh
sudo pacman -U dist/arch/makepkg/big-red-button-*.pkg.tar.zst
```

The package install hook reloads systemd, enables and starts
`big-red-buttond.service`, updates desktop/icon caches when available, and runs
`big-red-button-check --install-check`.

## 3. Check Installed Binaries

```bash
big-red-button version
big-red-button-gui -addr 127.0.0.1:0 -no-open
big-red-buttond -version
/usr/lib/big-red-button/wstunnel --version
big-red-button-check
```

## 4. Check The Daemon

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now big-red-buttond.service
sudo systemctl status big-red-buttond.service
```

Check daemon IPC:

```bash
big-red-button daemon-status
curl --unix-socket /run/big-red-button/launcher.sock \
  http://big-red-button/v1/status
```

## 5. Validate Profile Without Network Mutation

```bash
big-red-button validate-profile /path/to/profile.json
big-red-button-check --profile /path/to/profile.json
```

The check command validates the profile and runs Linux preflight with the
bundled WSTunnel helper.

## 6. Run Non-Mutating Smoke

```bash
/usr/share/doc/big-red-button/linux-smoke.sh \
  --profile /path/to/profile.json
```

## 7. Test The GUI

Launch **Big Red Button** from the application menu.

In the GUI:

1. Save Profile.
2. Leave `Tunnel helper binary` as `/usr/lib/big-red-button/wstunnel` unless
   you want to test another helper build.
3. Run `Preflight`.
4. Run `Connect`.
5. Check status.
6. Run `Disconnect`.
7. Run `Bundle` and keep the diagnostics archive.

## 8. CLI Real Connect Smoke

```bash
sudo big-red-button linux-connect \
  -yes \
  /path/to/profile.json

big-red-button status

sudo big-red-button linux-disconnect -yes
big-red-button status
```

## 9. Isolated App Smoke

Start with a harmless command:

```bash
sudo big-red-button linux-isolated-app \
  -yes \
  /path/to/profile.json -- /usr/bin/true

big-red-button isolated-sessions
sudo big-red-button linux-recover-isolated-sessions -yes -startup
```

Then test a real network command:

```bash
sudo big-red-button linux-isolated-app \
  -yes \
  /path/to/profile.json -- /usr/bin/curl https://ifconfig.me
```

## 10. Recovery And Diagnostics

If anything fails:

```bash
big-red-button diagnostics-bundle \
  -profile /path/to/profile.json \
  -output brb-diagnostics.tar.gz

sudo big-red-button linux-disconnect -yes
sudo big-red-button linux-recover-isolated-sessions -yes -all
```

## Minimum Pass Criteria

- daemon starts through systemd
- GUI opens from the desktop launcher
- profile validation succeeds
- preflight succeeds
- connect/disconnect completes without manual route or process cleanup
- diagnostics bundle is created
- isolated app smoke starts and recovers cleanly
