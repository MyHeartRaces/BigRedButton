# Roadmap

Date: 2026-04-25

## Phase 0: Bootstrap

- create the local repository
- record initial product statements
- fix the first implementation stack
- define the V7 profile schema consumed by the launcher
- scaffold the Go module and Wails v2 UI shell
- add a headless `tracegate-launcherctl` path before relying on the UI

## Phase 1: Linux MVP

Goal: one-button local client that can connect and disconnect a single V7
profile system-wide on Linux.

Scope:

- load one local V7 profile
- validate WireGuard and WSTunnel fields
- start `wstunnel client`
- start WireGuard
- add route exclusion for WSTunnel endpoint
- expose simple status to the UI
- cleanly disconnect and restore network state
- run the same lifecycle through `tracegate-launcherctl` for repeatable tests

Exit criteria:

- repeated connect/disconnect works without reboot
- WSTunnel server endpoint does not self-loop into WireGuard
- failed connect leaves no stale route or process
- disconnect restores baseline networking

## Phase 2: Windows Port and Tester Build

Goal: port the working Linux baseline to Windows and produce a tester build.

Scope:

- Windows helper/service
- WireGuard Windows integration
- `wstunnel.exe` lifecycle
- route exclusion and cleanup
- installer or repeatable manual install path
- minimal diagnostic bundle for testers

Exit criteria:

- testers can import one profile and use Connect/Disconnect
- disconnect restores routing
- failures produce enough diagnostic data to debug remotely

## Phase 3: macOS Port

Goal: port the stable desktop model to macOS.

Scope:

- macOS privileged helper
- WireGuardKit or approved equivalent
- WSTunnel process lifecycle
- route/DNS cleanup
- signing and notarization plan

## Deferred

- multi-profile manager
- secure profile vault
- account login
- advanced logs UI
- kill switch
- split tunneling UI
- automatic updates
- iOS and Android
