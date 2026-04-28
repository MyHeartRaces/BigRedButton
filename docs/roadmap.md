# Roadmap

Date: 2026-04-25

## Phase 0: Bootstrap

- create the local repository
- record initial product statements
- fix the first implementation stack
- record the development plan and quality gates
- define the VPN profile schema consumed by the launcher
- scaffold the Go module and desktop GUI shell
- add a headless `big-red-button` path before relying on the UI

## Phase 1: Linux MVP

Goal: one-button local client that can connect and disconnect a single VPN
profile system-wide on Linux.

Scope:

- load one local VPN profile
- validate WireGuard and WSTunnel fields
- start `wstunnel client`
- start WireGuard
- add route exclusion for WSTunnel endpoint
- expose simple status to the UI
- cleanly disconnect and restore network state
- run the same lifecycle through `big-red-button` for repeatable tests

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

## Phase 4: Isolated App Tunnel

Goal: route selected applications through an isolated VPN session without
changing host networking for the rest of the machine.

The requirements are recorded in
[`isolated-app-tunnel.md`](isolated-app-tunnel.md). They apply to Linux,
Windows and macOS even if the first prototype is Linux-only.

Scope:

- session UUID and app rule model
- helper-owned isolated session lifecycle
- session-only DNS handling
- session-only fail-closed behavior
- cleanup and crash recovery
- Linux network namespace backend prototype
- Linux isolated app plan and dry-run command sequence
- Windows service/WFP feasibility spike
- macOS Network Extension feasibility spike

Exit criteria:

- selected app traffic and DNS use the tunnel
- non-selected apps continue using the host network
- host default route and host DNS are unchanged
- tunnel failure does not leak selected app traffic outside VPN
- cleanup leaves no launcher-owned routes, interfaces, processes or rules

## Deferred

- multi-profile manager
- secure profile vault
- account login
- advanced logs UI
- kill switch
- split tunneling UI beyond isolated app tunnel
- automatic updates
- iOS and Android
