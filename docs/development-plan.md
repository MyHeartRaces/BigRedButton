# Development Plan

Date: 2026-04-25

Status: accepted direction

## Development Principle

Build the operational path before the UI.

The first usable product is one button, but the first implementation milestone
is a reliable headless lifecycle:

```text
load profile -> validate -> plan changes -> connect -> status -> disconnect -> cleanup
```

The desktop UI should call the same helper API that the CLI uses. Do not create a
separate UI-only connect path.

## Codebase Shape

Initial Go layout:

```text
cmd/
  big-red-button/       # CLI for development, diagnostics and tests
  big-red-buttond/         # privileged helper / daemon
internal/
  profile/                     # VPN profile parsing, validation, redaction
  engine/                      # connect/disconnect state machine
  supervisor/                  # child process lifecycle, especially wstunnel
  wireguard/                   # WireGuard adapter interfaces
  routes/                      # route planning and endpoint exclusion
  dns/                         # DNS plan, initially minimal
  ipc/                         # local API between UI, CLI and helper
  status/                      # state snapshots and health model
  platform/
    linux/                     # netlink, wgctrl and Linux command fallback
    windows/                   # stubs until Windows phase
    darwin/                    # stubs until macOS phase
testdata/
  profiles/                    # redacted VPN fixtures
frontend/                      # embedded desktop web UI, added after CLI lifecycle
```

The exact package names can change, but the separation must remain:

- profile model is platform-neutral
- lifecycle engine is platform-neutral
- privileged network operations are platform adapters
- UI never performs privileged operations directly

## Step 1: VPN Profile Contract

Create a launcher-owned VPN profile schema based on server export
`effective_config_json`.

Minimum required fields:

- profile name: `WGWS-Direct`
- WSTunnel URL: a real `wss://...:443/...` target from the export payload
- WSTunnel local UDP endpoint
- WireGuard private key
- WireGuard public server key
- optional preshared key
- WireGuard tunnel address
- client route AllowedIPs
- DNS value if provided
- MTU
- persistent keepalive

Rules:

- reject placeholders such as `REPLACE_*`
- reject non-`wss` WSTunnel URLs
- reject non-443 WSTunnel URLs in MVP
- reject non-loopback local UDP endpoints
- reject MTU outside the accepted conservative range
- normalize profile data before it reaches the engine
- redact secrets in all status and diagnostic output

Exit criteria:

- unit tests cover valid and invalid profile fixtures
- CLI can run `validate-profile <path>`
- no network privileges are needed for this step

## Step 2: Lifecycle Engine With Dry Run

Implement the connect/disconnect state machine without touching the host
network yet.

The engine should produce an explicit plan:

- resolve WSTunnel host
- identify current default route gateway/interface
- add route exclusion for WSTunnel endpoint
- start WSTunnel
- create or configure WireGuard interface
- apply WireGuard route/DNS plan
- verify basic status

The disconnect plan must be equally explicit:

- stop WireGuard or remove the temporary interface
- stop WSTunnel
- remove only routes created by the launcher
- return a final status

Exit criteria:

- dry-run connect/disconnect prints deterministic plans
- every step has a rollback action where applicable
- repeated connect when already connected is idempotent
- repeated disconnect when already disconnected is idempotent

## Step 3: Linux Helper MVP

Implement the privileged Linux backend.

Preferred path:

- kernel WireGuard
- netlink for interface and route operations
- `wgctrl-go` where practical for WireGuard configuration
- supervised `wstunnel client` process
- temporary `ip` / `wg` command fallback only where it unblocks the MVP

Required behavior:

- route the WSTunnel server endpoint outside WireGuard before full-tunnel
  routes are applied
- fail closed for the attempted connection if route exclusion cannot be
  established
- clean up partially applied state on failure
- keep launcher-created state identifiable for cleanup

Exit criteria:

- `big-red-button connect --profile <path>` works on Linux
- `big-red-button status` reports connected/disconnected/failure
- `big-red-button disconnect` restores the previous route state
- failures do not leave an orphaned WSTunnel process
- failures do not leave stale launcher-owned routes

## Step 4: Linux Test Harness

Add tests before adding the GUI.

Test layers:

- unit tests for profile validation and redaction
- unit tests for route planning
- fake platform tests for rollback order
- optional privileged Linux integration tests
- manual smoke script for a real VPN profile

The privileged integration tests should be opt-in and clearly marked so they do
not run accidentally on a developer machine.

Exit criteria:

- ordinary `go test ./...` runs without root
- privileged tests require an explicit environment flag
- manual smoke test has a repeatable command sequence

## Step 5: Minimal desktop UI

Add UI only after the CLI lifecycle works.

UI scope:

- select or import one profile
- Connect / Disconnect
- show current state
- show last error

The UI talks to `big-red-buttond` over local IPC. It must not call route,
WireGuard or process-management code directly.

Exit criteria:

- the button uses the same helper lifecycle as the CLI
- closing the UI does not leave the helper in an undefined state
- reconnect after UI restart works

## Step 6: Linux Packaging

Package only after lifecycle and UI are stable.

Linux packaging scope:

- install `big-red-button`
- install `big-red-buttond`
- install or locate bundled `wstunnel`
- set up helper privileges through the chosen model
- provide uninstall cleanup

For the first Linux MVP, a documented manual install path is acceptable before
full `.deb` / `.rpm` packaging.

## Step 7: Windows Port

Do not start this until Linux exit criteria are met.

Windows port scope:

- keep Go profile model and lifecycle engine
- implement Windows service/helper
- use official WireGuard Windows tunnel service or embeddable service
- supervise `wstunnel.exe`
- implement route exclusion with Windows APIs or a narrow native shim
- add tester diagnostic bundle

Windows testing must account for the fact that the primary developer may not
have local Windows hardware. At minimum, use CI smoke tests for non-privileged
parts and arrange tester/VM coverage for real tunnel lifecycle.

## Step 8: macOS Port

macOS follows Windows.

Expected shared parts:

- profile parser
- lifecycle engine
- WSTunnel supervision model
- status model
- UI structure

Expected platform-specific parts:

- privileged helper
- tunnel implementation
- route/DNS handling
- signing and notarization

## Do Not Do Yet

- multi-profile UI
- account login
- automatic updates
- kill switch
- split tunneling controls
- advanced logs screen
- mobile app
- custom WireGuard implementation
- custom WSTunnel implementation
