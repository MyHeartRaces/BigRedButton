# Initial Statements

Date: 2026-04-25

Project name: Big Red Button.

## Product

Big Red Button is a desktop client for Tracegate V7, where V7 means
WireGuard over WSTunnel:

```text
client WireGuard -> local UDP endpoint -> WSTunnel WSS -> Transit WSTunnel -> server WireGuard
```

The client should make V7 usable for a normal desktop user through a minimal
connect/disconnect workflow.

## Platform Priority

The platform order is:

1. Linux
2. Windows
3. macOS
4. iOS and Android

Linux is the first implementation platform. Windows comes next and becomes the
first tester-distribution target. macOS follows after Windows. Mobile platforms
are explicitly deferred.

## Porting Rule

Do not begin a platform port until the previous target has a working baseline.

A working baseline means:

- profile import or local profile loading works
- WSTunnel starts and stops cleanly
- WireGuard starts and stops cleanly
- full-tunnel routing works
- the WSTunnel server endpoint is routed outside the tunnel
- disconnect restores the previous network state
- common error cases produce actionable status

## MVP Boundary

The first MVP is a simple-button client:

- Connect
- Disconnect
- show current state
- show the last failure in plain language

The MVP does not include:

- multi-profile management
- cloud account login
- in-app purchase or subscription flows
- advanced log viewer
- kill switch
- split tunneling controls
- automatic updates
- mobile builds

## System-Wide Target

System-wide connectivity requires a TUN/VPN interface. A proxy-only client can
cover only applications that honor HTTP/SOCKS proxy settings, so it is not a
replacement for the main VPN mode.

Proxy mode may be added as a fallback or diagnostic mode after the VPN path is
stable.

## Quality Gate

Portability is not the main success metric. A port is acceptable only if it
preserves the connect/disconnect semantics and does not leave broken routes,
orphaned processes or stale privileged state.

