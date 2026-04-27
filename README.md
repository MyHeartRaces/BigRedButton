# Tracegate Launcher

Tracegate Launcher is the planned desktop client for Tracegate V7:
WireGuard over WSTunnel.

Status: headless implementation scaffold. No production desktop client or
privileged network helper exists in this repository yet.

## Current Product Decision

- Build the first working client on Linux.
- Do not port before the Linux client can reliably connect, disconnect and
  recover from ordinary failures.
- After Linux is working, port to Windows and give that build to testers.
- Treat macOS as the next desktop port after Windows validation. The UI and
  core model should port cleanly, but the privileged networking layer still
  requires platform-specific validation.
- Keep iOS and Android out of scope until the desktop client is stable.

## Fixed Stack

- Go for the operational core, privileged helper and platform networking
  adapters.
- Wails v2 for the desktop shell.
- TypeScript + Svelte for the small one-button UI.
- Bundled upstream `wstunnel` binaries per target platform.
- Native platform adapters only where Go cannot own the integration cleanly.
  C++ is allowed for narrow shims, not as the default application runtime.

## First MVP

The first MVP is intentionally narrow:

- one profile
- one Connect / Disconnect control
- system-wide tunnel as the target behavior
- WireGuard over local WSTunnel
- route exclusion for the WSTunnel server endpoint
- minimal status reporting
- no multi-profile UI
- no account management
- no advanced logs UI
- no kill switch
- no split tunneling UI
- no mobile port

Proxy-only mode can exist later as a diagnostic or fallback mode, but it is not
the product target because it cannot cover all system traffic.

## Current Implementation

The first code slice is intentionally headless:

```bash
go test ./...
go run ./cmd/tracegate-launcherctl validate-profile testdata/profiles/valid-v7.json
go run ./cmd/tracegate-launcherctl plan-connect -endpoint-ip 203.0.113.10 testdata/profiles/valid-v7.json
go run ./cmd/tracegate-launcherctl plan-disconnect
go run ./cmd/tracegate-launcherctl linux-dry-run-connect -endpoint-ip 203.0.113.10 -default-gateway 192.0.2.1 -default-interface eth0 testdata/profiles/valid-v7.json
go run ./cmd/tracegate-launcherctl linux-dry-run-disconnect -runtime-root /run/tracegate-launcher
```

Implemented so far:

- Go module
- V7 profile parser and validator
- secret-redacted profile summary
- `tracegate-launcherctl validate-profile`
- `tracegate-launcherctl plan-connect`
- `tracegate-launcherctl plan-disconnect`
- secret-free dry-run connect/disconnect plans
- lifecycle engine with fake-executor rollback tests
- platform-neutral route exclusion model
- Linux `ip route get` output parser for future helper integration
- Linux `ip route` command builders for route exclusion apply/rollback
- Linux read-only endpoint route discovery through an injectable command runner
- Linux dry-run connect executor exposed through `tracegate-launcherctl`,
  with optional read-only route discovery
- secret-free runtime state model and file store for future disconnect/rollback
- Linux dry-run disconnect can read runtime state and plan saved route cleanup
- real Linux route executor for endpoint exclusions, covered by fake-runner tests
- WSTunnel client command builder for WireGuard UDP forwarding
- WSTunnel process runner/executor abstraction with rollback tests
- WireGuard `wg setconf` renderer and Linux command builders
- Linux WireGuard executor with temporary secret config cleanup
- redacted valid and invalid fixtures

## Repository Layout

- `docs/statements.md`: initial product and platform statements.
- `docs/stack.md`: fixed implementation stack.
- `docs/architecture.md`: first-pass architecture and stack notes.
- `docs/application-architecture.md`: detailed process, IPC, lifecycle and
  platform architecture.
- `docs/development-plan.md`: next development sequence and quality gates.
- `docs/roadmap.md`: phased delivery plan.
