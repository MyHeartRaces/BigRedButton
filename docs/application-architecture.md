# Application Architecture

Date: 2026-04-25

Status: accepted baseline

## Purpose

Big Red Button is a desktop launcher for one VPN profile:
WireGuard over WSTunnel.

The application does not implement WireGuard or WSTunnel. It owns the reliable
local lifecycle around existing components:

```text
validate profile
resolve WSTunnel endpoint
protect WSTunnel endpoint from tunnel self-loop
start WSTunnel
configure WireGuard
report status
disconnect and clean up only launcher-owned state
```

## Top-Level Shape

```text
user session

+----------------------+       +----------------------+
| big-red-button-gui   |       | big-red-button       |
| desktop web UI       |       | CLI / diagnostics    |
+----------+-----------+       +----------+-----------+
           | local IPC                    | local IPC
           +---------------+--------------+
                           |
                 privileged boundary
                           |
                 +---------v----------+
                 | big-red-buttond|
                 | Go helper / daemon |
                 +----+----------+----+
                      |          |
        +-------------+          +-------------+
        |                                      |
  +-----v------+     +--------------+     +----v-----+
  | WSTunnel   |     | WireGuard    |     | Routes   |
  | process    |     | adapter      |     | and DNS  |
  +------------+     +--------------+     +----------+
```

The UI and CLI are clients of the helper. They do not own privileged network
operations.

## Deliverables

### `big-red-button-gui`

Desktop UI launcher built in Go with embedded HTML/CSS/JavaScript.

Responsibilities:

- present one selected profile
- expose Connect / Disconnect
- display current status
- display last failure
- call helper API over local IPC when the helper exists
- call the guarded Linux CLI through `pkexec` during the first GUI MVP

Non-responsibilities:

- no route changes
- no WireGuard configuration
- no WSTunnel process ownership
- no root / Administrator runtime

### `big-red-buttond`

Privileged Go helper / daemon.

Responsibilities:

- accept profile payloads over local IPC
- validate and normalize profiles before use
- own the connect/disconnect state machine
- start and stop WSTunnel
- configure and tear down WireGuard
- add and remove launcher-owned routes
- keep runtime state for rollback and diagnostics
- redact secrets in all responses

The helper is the only component allowed to mutate system networking.

### `big-red-button`

Go CLI for development, diagnostics and headless testing.

Responsibilities:

- validate profile files
- print dry-run plans
- call connect/disconnect/status helper APIs
- produce diagnostic bundles

The CLI must exercise the same helper API as the UI. It is not a separate
implementation path.

## Core Go Packages

Initial internal package boundaries:

```text
internal/profile      parse, normalize, validate and redact VPN profiles
internal/engine       connection state machine and rollback model
internal/planner      connect/disconnect plans and dry-run rendering
internal/supervisor   child process lifecycle, especially WSTunnel
internal/wireguard    WireGuard adapter interfaces and shared types
internal/routes       route model, route ownership and endpoint exclusion
internal/dns          DNS plan and platform adapter interfaces
internal/ipc          local helper API
internal/status       state snapshots, health and diagnostics
internal/platform     Linux, Windows and macOS implementations
```

The `profile`, `engine`, `planner`, `ipc` and `status` packages should remain
platform-neutral. Linux/Windows/macOS behavior belongs below `platform`.

## Profile Boundary

The launcher consumes a normalized VPN profile derived from server
`effective_config_json`.

Minimum profile fields:

- profile name: `WGWS-Direct`
- WSTunnel URL: `wss://host:443/path`
- WSTunnel local UDP listen endpoint
- WireGuard client private key
- WireGuard server public key
- optional WireGuard preshared key
- WireGuard tunnel address
- client route AllowedIPs
- optional DNS value
- MTU
- persistent keepalive

Validation rules:

- reject `REPLACE_*` placeholders
- require `wss://`
- require port `443` in the MVP
- reject query, fragment and whitespace in WSTunnel URL
- require local UDP listen to be loopback
- require MTU within the supported range
- require keepalive within the supported range
- redact all keys in status, logs and diagnostic output

The UI should read the profile file and send profile bytes to the helper. The
helper should not accept arbitrary user file paths as privileged input.

## IPC Boundary

Linux MVP IPC:

- Unix domain socket under `/run/big-red-button/launcher.sock`
- helper-owned socket permissions
- request/response protocol in JSON for early development
- versioned message envelope

Current implementation slice:

- `big-red-buttond` listens on the Unix socket.
- `/v1/health`, `/v1/status` and `/v1/diagnostics` are read-only JSON
  endpoints.
- `big-red-button daemon-status` can query the daemon over the same socket for
  smoke testing and scripted diagnostics.
- Mutating operations still use the guarded CLI path through `pkexec`.

Initial API:

- `ValidateProfile`
- `PlanConnect`
- `Connect`
- `Disconnect`
- `Status`
- `Diagnostics`

Future platform mapping:

- Windows: named pipe with strict local ACLs
- macOS: Unix domain socket first, XPC only if the helper model requires it

## Runtime State

The helper stores volatile runtime state under `/run/big-red-button`.

State should include:

- active profile fingerprint, not raw secrets
- resolved WSTunnel endpoint IPs
- launcher-owned routes
- WSTunnel process metadata
- WireGuard interface name
- current state machine state
- last sanitized error

Do not persist profile secrets in MVP. Persistent profile storage is a later
feature and must have its own storage/security design.

## State Machine

Connection states:

```text
Idle
  -> Validating
  -> Planning
  -> Connecting
  -> Connected
  -> Disconnecting
  -> Idle

Any active state
  -> FailedRecoverable
  -> Disconnecting
  -> Idle

Failure with incomplete cleanup
  -> FailedDirty
```

`FailedDirty` means the helper believes some launcher-owned route, process or
interface may still exist. The UI must show this differently from an ordinary
connect failure.

Connect must be idempotent:

- connecting with the same active profile returns current status
- connecting with a different profile fails until disconnect succeeds

Disconnect must be idempotent:

- disconnect from `Idle` returns success
- disconnect from `FailedRecoverable` attempts cleanup
- disconnect from `FailedDirty` attempts best-effort cleanup and reports what
  remains unknown

## Connect Pipeline

The helper executes connect in this order:

1. Parse and validate profile.
2. Resolve WSTunnel hostname before tunnel routes are applied.
3. Validate platform prerequisites before mutating network state.
4. Snapshot current route to each resolved WSTunnel endpoint IP.
5. Build route exclusion plan for all resolved endpoint IPs.
6. Reserve local runtime names and ports.
7. Start WSTunnel client and wait until the local UDP endpoint is ready.
8. Create/configure the WireGuard interface.
9. Apply WireGuard address, peer, MTU and keepalive.
10. Apply route plan for client AllowedIPs.
11. Apply DNS plan if supported by the current platform adapter.
12. Verify WSTunnel process is alive and WireGuard interface exists.
13. Store runtime state and return `Connected`.

If any step fails, the engine runs rollback in reverse order for completed
steps.

## Disconnect Pipeline

The helper executes disconnect in this order:

1. Read runtime state.
2. Remove launcher-owned DNS changes, if any.
3. Remove launcher-owned WireGuard routes.
4. Remove or bring down the launcher WireGuard interface.
5. Stop WSTunnel process.
6. Remove launcher-owned WSTunnel endpoint routes.
7. Clear runtime state.
8. Return `Idle`.

Cleanup must remove only state created by Big Red Button.

## WSTunnel Supervision

The WSTunnel adapter owns:

- binary discovery or bundled binary path
- command construction
- process start
- readiness check
- stdout/stderr capture with redaction
- process stop with timeout
- forced kill only after graceful stop fails

The MVP uses upstream `wstunnel` as an external binary. The launcher does not
fork or reimplement it.

## WireGuard Adapter

Linux MVP target:

- kernel WireGuard
- create/configure interface from Go
- use netlink for interface/address/route operations where possible
- use `wgctrl-go` where practical for WireGuard peer state
- allow `ip` / `wg` command fallback to unblock the MVP
- avoid `wg-quick` as the primary engine because hidden route/DNS behavior
  makes deterministic rollback harder

Windows target:

- keep shared Go profile and lifecycle engine
- use official WireGuard Windows tunnel service or embeddable service
- add a narrow native shim only if the Go integration is not robust enough

macOS target:

- keep shared Go profile and lifecycle engine where possible
- evaluate WireGuardKit / Network Extension for the tunnel layer
- expect a native helper/shim for Apple-specific APIs

## Route Architecture

Route handling is a first-class part of the product, not a side effect.

Mandatory route behavior:

- resolve WSTunnel host before full-tunnel routes are active
- add host routes for each WSTunnel endpoint IP through the pre-tunnel gateway
- never route the WSTunnel endpoint through WireGuard
- record every launcher-owned route in runtime state
- remove only recorded launcher-owned routes during disconnect

The first MVP should support IPv4 route exclusion. IPv6 route exclusion should
be planned explicitly and either supported or rejected with a clear validation
error before production use.

## DNS Architecture

DNS handling is explicit and platform-owned.

Rules:

- parse DNS values from the profile
- include DNS commands in dry-run output
- record applied launcher-owned DNS state in runtime state
- restore only launcher-owned DNS state during disconnect and rollback
- do not silently rewrite system DNS through an unknown mechanism
- keep isolated app DNS scoped to the isolated session, not host DNS

Linux system-wide mode uses the `systemd-resolved` link API through
`resolvectl`:

- `resolvectl dns <wireguard-iface> <server...>`
- `resolvectl domain <wireguard-iface> ~.`
- `resolvectl default-route <wireguard-iface> yes`
- `resolvectl revert <wireguard-iface>` during disconnect and rollback

Linux isolated app mode does not use host DNS. It writes namespace-local DNS
state under `/etc/netns/<namespace>/resolv.conf`.

Windows and macOS must implement their own DNS adapters. They should preserve
the same invariants: explicit apply, recorded launcher ownership, deterministic
restore, and no global DNS mutation for isolated app mode.

## Security Boundaries

- UI and CLI are unprivileged by default.
- Helper owns all privileged operations.
- Helper does not accept arbitrary filesystem paths as root-owned actions.
- Secrets are redacted before logs, status and diagnostics.
- Profile persistence is deferred until a storage design exists.
- Local IPC must authenticate the local caller at least by OS identity.
- No shell interpolation for profile values.
- Command fallbacks must pass arguments as argv, never through a shell.

## Diagnostics

Diagnostics should be useful without leaking secrets.

Initial diagnostic bundle:

- launcher version
- OS and platform adapter
- current sanitized status
- active profile fingerprint
- WSTunnel URL host/path without keys
- resolved endpoint IPs
- route plan and applied route records
- WireGuard interface name and public peer fingerprint
- recent sanitized helper events
- recent sanitized WSTunnel process events

On Linux, status marks runtime state dirty when a saved launcher-owned process
PID no longer exists. System-wide mode checks the WSTunnel process. Isolated app
mode checks both the selected app process and the WSTunnel control process.

Raw private keys, preshared keys and full profile payloads must never appear in
diagnostics.

## Testing Architecture

Default tests must not require root.

Test layers:

- profile parser and validator tests
- redaction tests
- route planner tests with fake route tables
- engine rollback tests with fake platform adapters
- supervisor tests with short-lived fake processes
- IPC contract tests
- opt-in privileged Linux integration tests

Privileged tests require an explicit environment flag and should never run as
part of the default local test command.

## First Implementation Slice

Implement in this order:

1. Go module and package skeleton.
2. VPN profile model, validation and redaction.
3. `big-red-button validate-profile`.
4. Planner and dry-run connect/disconnect output.
5. Fake platform adapter tests for rollback.
6. Linux route planner and endpoint exclusion.
7. WSTunnel supervisor.
8. Linux WireGuard adapter.
9. Helper IPC.
10. Minimal desktop UI.
