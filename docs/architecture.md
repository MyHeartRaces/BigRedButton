# Architecture Notes

Date: 2026-04-25

## Preferred Shape

Tracegate Launcher should be split into a small UI and a privileged networking
backend.

```text
Tracegate Launcher UI
        |
        | local IPC
        v
Tracegate Launcher helper
        |
        +-- profile parser and validator
        +-- WSTunnel process manager
        +-- WireGuard backend adapter
        +-- route and DNS adapter
        +-- status and health reporter
```

The UI must not run as root or Administrator during ordinary use. Privileged
operations belong in the helper.

## Provisional Stack

Default candidate:

- Rust for core logic and privileged helper
- Tauri 2 for the desktop shell
- TypeScript for the small UI
- bundled `wstunnel` binary per target platform
- platform-specific WireGuard adapters

Acceptable alternative:

- C++/Qt UI and helper if native desktop control becomes more important than
  development speed

Not preferred for production:

- Python as the main desktop runtime

Python can still be useful for tooling, fixture generation and integration
tests.

## Linux Backend

The first backend should target Linux.

Likely implementation path:

- use kernel WireGuard via `wg` / `wg-quick` initially, then move to netlink or
  a library-backed implementation if needed
- manage `wstunnel client` as a child process
- add an explicit host route for the resolved WSTunnel server endpoint before
  enabling full-tunnel routing
- restore routes and stop processes on disconnect
- support systemd integration only after the manual lifecycle is stable

## Windows Backend

Windows is the second platform and the first external tester target.

Likely implementation path:

- use the official WireGuard Windows tunnel service or embeddable service
- manage `wstunnel.exe` as a helper-owned child process or service dependency
- use Windows route APIs for endpoint exclusion and cleanup
- communicate with the UI through a named pipe or equivalent local IPC with
  strict ACLs

## macOS Backend

macOS is expected to share most UI and profile logic with Linux/Windows, but
the networking layer remains platform-specific.

Likely implementation path:

- evaluate WireGuardKit / Network Extension for production quality
- use a launchd helper for privileged operations
- validate route and DNS cleanup separately from Linux

## Core Invariants

- Never route the WSTunnel server endpoint through the WireGuard tunnel.
- Never leave WSTunnel or WireGuard processes running after disconnect.
- Never persist raw profile material outside the agreed local storage boundary.
- Keep the first client single-profile until connect/disconnect is reliable.
- Treat proxy-only mode as a fallback, not the primary product path.

