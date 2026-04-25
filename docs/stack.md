# Stack Decision

Date: 2026-04-25

Status: accepted

## Decision

Tracegate Launcher uses Go as the main operational language.

The fixed stack is:

- Go for the operational core, privileged helper, process supervision,
  platform networking adapters and CLI tooling
- Wails v2 for the desktop shell
- TypeScript + Svelte for the UI
- upstream `wstunnel` as a bundled external binary
- platform-native shims only when the OS integration cannot be owned cleanly in
  Go

## Why Go

The hard part of Tracegate Launcher is operational control, not UI complexity.
The client must manage privileged networking, child processes, route cleanup,
WireGuard state and diagnostics. Go is a better default than C++ for the first
implementation because it gives:

- simple static binaries
- fast Linux-first iteration
- good process and service lifecycle primitives
- direct access to Linux networking libraries
- practical Windows service support later
- simpler memory safety and concurrency than C++

C++ remains acceptable for a narrow native adapter, but not as the primary
runtime unless Go becomes a concrete blocker.

## UI Stack

The UI is intentionally small and should remain replaceable.

Default UI stack:

- Wails v2
- TypeScript
- Svelte

The UI owns:

- profile selection/import entry point
- Connect / Disconnect button
- current state display
- last failure display

The UI does not own privileged operations and must not run as root or
Administrator during normal use.

## Process Boundary

The application is split into three deliverables:

- `tracegate-launcher`: desktop UI
- `tracegate-launcherd`: privileged helper / daemon
- `tracegate-launcherctl`: CLI for development, diagnostics and headless tests

The UI talks to the helper over local IPC:

- Linux: Unix domain socket
- Windows: named pipe
- macOS: Unix domain socket or XPC, depending on the helper model

## Linux Implementation Stack

Linux is the first implementation target.

Preferred Linux backend:

- kernel WireGuard
- Go helper running with required privileges
- Go route/interface management through netlink
- WireGuard peer/config control through Go libraries where practical
- controlled fallback to `ip` / `wg` commands only for early MVP gaps
- `wstunnel client` supervised as a child process

The route exclusion for the WSTunnel server endpoint is mandatory and belongs
in the helper.

## Windows Implementation Stack

Windows is the second target and first tester-distribution target.

Preferred Windows backend:

- Go helper installed as a Windows service
- official WireGuard Windows tunnel service or embeddable service
- `wstunnel.exe` supervised by the helper
- route management through Windows APIs or a narrow native shim
- named-pipe IPC with strict local ACLs

C++ is allowed here only if direct Windows networking or WireGuard integration
requires it. The first port should still keep the orchestration and profile
model in Go.

## macOS Implementation Stack

macOS follows after Windows validation.

Preferred macOS backend:

- shared Go profile model and orchestration where possible
- launchd helper or approved privileged helper model
- WireGuardKit / Network Extension evaluation for the tunnel layer
- native shim if required by Apple APIs

Do not assume Linux route and DNS behavior will carry over unchanged.

## Explicit Non-Goals

- no Python production runtime
- no Electron shell
- no C++ full application rewrite unless Go becomes a proven blocker
- no custom WireGuard implementation
- no custom WSTunnel implementation

