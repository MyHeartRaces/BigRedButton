# Architecture Notes

Date: 2026-04-25

Detailed implementation architecture is recorded in
[`application-architecture.md`](application-architecture.md). This file keeps
the short architectural summary.

Isolated per-application tunnel requirements are recorded separately in
[`isolated-app-tunnel.md`](isolated-app-tunnel.md). Those requirements are
platform-neutral: Linux may be implemented first, but Windows and macOS must
preserve the same safety and isolation semantics.

## Preferred Shape

Big Red Button should be split into a small UI and a privileged networking
backend.

```text
Big Red Button UI
        |
        | local IPC
        v
Big Red Button helper
        |
        +-- profile parser and validator
        +-- WSTunnel process manager
        +-- WireGuard backend adapter
        +-- route and DNS adapter
        +-- status and health reporter
```

The UI must not run as root or Administrator during ordinary use. Privileged
operations belong in the helper.

## Fixed Stack

Accepted stack:

- Go for core logic, privileged helper, CLI tooling and networking adapters
- Go standard-library desktop web launcher for the first GUI MVP
- embedded HTML/CSS/JavaScript for the small UI
- bundled `wstunnel` binary per target platform
- platform-specific WireGuard, route and DNS adapters

C++ policy:

- allowed only as a narrow native shim where Go cannot own the platform API
  cleanly
- not the default UI or operational runtime

Not preferred for production:

- Python as the main desktop runtime
- Electron as the desktop shell
- a custom WireGuard implementation
- a custom WSTunnel implementation

Python can still be useful for tooling, fixture generation and integration
tests.

## Deliverables

- `big-red-button-gui`: desktop GUI launcher.
- `big-red-buttond`: privileged Go helper / daemon.
- `big-red-button`: Go CLI for development, diagnostics and headless
  tests.

## Linux Backend

The first backend should target Linux.

Likely implementation path:

- use kernel WireGuard
- manage interfaces and routes from Go, preferably through netlink
- control WireGuard state from Go where practical
- allow temporary `ip` / `wg` command adapters only to unblock the earliest MVP
- supervise `wstunnel client` as a child process
- add an explicit host route for the resolved WSTunnel server endpoint before
  enabling full-tunnel routing
- restore routes and stop processes on disconnect
- support systemd integration only after the manual lifecycle is stable

For isolated app tunnel mode, the Linux backend uses a per-session network
namespace with WireGuard inside the namespace and a host-side WSTunnel control
process bound to the session `veth` address. This keeps host default routes and
host DNS untouched while letting the namespace fail closed.

## Windows Backend

Windows is the second platform and the first external tester target.

Likely implementation path:

- install a Go helper as a Windows service
- use the official WireGuard Windows tunnel service or embeddable service
- manage `wstunnel.exe` as a helper-owned child process or service dependency
- use Windows route APIs for endpoint exclusion and cleanup, with a C++ shim
  only if direct Go integration is not robust enough
- communicate with the UI through a named pipe or equivalent local IPC with
  strict ACLs

## macOS Backend

macOS is expected to share most UI and profile logic with Linux/Windows, but
the networking layer remains platform-specific.

Likely implementation path:

- reuse Go profile parsing and orchestration where practical
- evaluate WireGuardKit / Network Extension for production quality
- use a launchd helper for privileged operations
- validate route and DNS cleanup separately from Linux

## Core Invariants

- Never route the WSTunnel server endpoint through the WireGuard tunnel.
- Never leave WSTunnel or WireGuard processes running after disconnect.
- Never persist raw profile material outside the agreed local storage boundary.
- Keep the first client single-profile until connect/disconnect is reliable.
- Treat proxy-only mode as a fallback, not the primary product path.
- For isolated app tunnel mode, never change host default routes or host DNS;
  all isolation and kill-switch behavior must be scoped to the app session.
