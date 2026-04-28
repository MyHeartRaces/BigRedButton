# Isolated App Tunnel Requirements

Date: 2026-04-28

Status: accepted product and architecture requirement

## Intent

Big Red Button should support an isolated per-application tunnel mode in
addition to the system-wide VPN mode.

This feature is not a proxy-only mode. It is an isolated app network session:
selected applications run through a VPN tunnel, while the host network and all
non-selected applications keep using the ordinary network path.

The requirements in this document are platform-neutral. Linux may be the first
implementation backend, but the same behavior and safety expectations apply to
Windows and macOS. Windows is especially important because it is the first
external tester platform.

## Product Requirements

- A tunnel session has a stable internal UUID.
- A user can bind an app rule to that UUID.
- The app rule must identify the selected application and its child processes,
  not just one transient PID.
- Traffic from selected applications must use the tunnel.
- Traffic from non-selected applications must stay outside the tunnel.
- If the tunnel fails, selected applications must fail closed instead of
  falling back to the host network.
- The host default route must not be changed for isolated app mode.
- The host DNS configuration must not be changed for isolated app mode.
- Host applications such as Steam, browsers and system updaters must keep
  working normally when they are not part of the tunnel session.
- The mode must be suitable for latency-sensitive applications such as games,
  subject to the limits of the underlying transport.

## Safety Invariants

These invariants apply to every platform backend:

- Do not mutate global host routing for isolated app mode.
- Do not mutate global host DNS for isolated app mode.
- Do not install an unscoped global kill switch.
- Do not let selected app traffic leak outside the tunnel.
- Do not let DNS from selected apps leak outside the tunnel.
- Do not leave orphaned routes, interfaces, processes, firewall rules or state
  records after disconnect, app exit or crash.
- Do not run the GUI as root or Administrator.
- Do not expose a helper API that can run arbitrary privileged commands.
- Tag every privileged object with the session UUID or an equivalent
  launcher-owned identifier where the platform supports it.

## DNS Requirements

DNS is part of the isolated session, not host configuration.

- The host resolver stays untouched.
- The isolated session has its own resolver configuration or resolver policy.
- Tunnel endpoint resolution must happen before tunnel routes are active, or
  through a control path that cannot self-loop through the tunnel.
- If per-session DNS isolation is unavailable on a platform, isolated app mode
  must be blocked or clearly marked unsupported on that platform.

## Quality Requirements

The isolated tunnel must not reduce network quality for non-selected
applications on the host. The implementation should measure and expose:

- tunnel helper process health
- WireGuard interface health
- latest handshake age where available
- endpoint route health
- MTU and effective tunnel path configuration
- last sanitized error

WGWS can add latency, jitter and head-of-line blocking compared to plain UDP
WireGuard. The architecture must not add avoidable extra global routing or DNS
side effects on top of that.

## Session Lifecycle

The helper owns the session lifecycle:

```text
CreateSession
  -> PrepareIsolation
  -> ResolveEndpoint
  -> StartTunnelHelper
  -> StartWireGuard
  -> ApplySessionOnlyKillSwitch
  -> LaunchApp
  -> MonitorProcessTree
  -> Cleanup
```

Session states:

```text
Creating
Connecting
Connected
Degraded
Disconnecting
Failed
FailedDirty
Closed
```

`FailedDirty` means the helper believes some launcher-owned session state may
remain and should be surfaced clearly in the UI.

The MVP may allow only one isolated app session at a time, but the state model
must include a session UUID from the start.

## Linux Backend

The preferred Linux backend is network namespace isolation:

- create a per-session network namespace
- create a scoped `veth` pair between host and namespace
- run the tunnel helper inside the namespace or route its data plane through
  namespace-local interfaces
- configure WireGuard inside the namespace
- set the namespace default route through WireGuard
- configure DNS through namespace-local resolver state
- apply the kill switch only inside the namespace
- launch and monitor the selected app process tree inside the namespace
- clean up the namespace, veth pair, processes and rules on exit

The root namespace should remain untouched except for minimal, tagged
forwarding/NAT/control-path objects required for the specific session.

The alternate UID-based design is an acceptable fallback only if network
namespace integration blocks the first Linux prototype. It is weaker because it
depends on launching the application under a different user and does not give
the same isolation boundary.

## Windows Backend

Windows must not be treated as a route-table-only port of Linux.

Expected Windows shape:

- unprivileged GUI
- Windows service as the privileged helper
- strict local IPC, for example a named pipe with local ACLs
- app rules based on executable path, package identity, process ID, process
  tree and optionally binary signature or hash
- internal app profile UUID mapped to those Windows identities
- Windows Filtering Platform for process-aware traffic classification and
  enforcement
- WireGuard/Wintun integration owned by the service
- DNS policy scoped to the selected app/session, not global system DNS
- fail-closed behavior for selected apps when the tunnel is unavailable

Anti-cheat, launchers, subprocesses and packaged applications must be tested as
first-class cases. A rule that captures only the initial PID is not enough for
games or complex desktop apps.

## macOS Backend

macOS must satisfy the same product invariants, but it cannot mirror Linux
network namespaces.

Expected macOS shape:

- unprivileged GUI
- launchd-managed privileged helper or Network Extension host app
- platform-specific network implementation through Network Extension where
  required
- app identity based on bundle ID, executable path, code signature and process
  tree where available
- no global DNS rewrite for isolated app mode
- fail-closed behavior for selected apps

Per-app VPN behavior on macOS may require Apple entitlements and may have
distribution constraints. Those constraints must be discovered early before
promising parity in public releases.

## Helper API Direction

The helper API should expose high-level operations only:

- `CreateIsolatedSession`
- `PlanIsolatedSession`
- `LaunchAppInSession`
- `StopIsolatedSession`
- `IsolatedSessionStatus`
- `CollectDiagnostics`

The API must not expose raw privileged command execution.

## Acceptance Criteria

An isolated app tunnel implementation is acceptable only when:

- selected app traffic uses the tunnel
- selected app DNS uses the session DNS path
- selected app traffic fails closed when the tunnel is down
- non-selected apps keep their existing network path
- host DNS remains unchanged
- host default route remains unchanged
- disconnect and crash recovery clean up launcher-owned state
- diagnostics can explain failures without leaking secrets
- platform-specific limitations are explicit in the UI and documentation
