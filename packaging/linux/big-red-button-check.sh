#!/usr/bin/env bash
set -u

profile=""
install_check=0

usage() {
  cat <<'USAGE'
usage: big-red-button-check [--profile profile.json] [--install-check]

Checks the Big Red Button Linux installation. Without --profile it verifies the
installed binaries, bundled WSTunnel helper, desktop registration and daemon.
With --profile it also validates the VPN profile and runs Linux preflight.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile)
      profile="${2:-}"
      shift 2
      ;;
    --install-check)
      install_check=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

status=0
wstunnel="/usr/lib/big-red-button/wstunnel"

ok() {
  printf 'ok: %s\n' "$*"
}

fail() {
  printf 'missing: %s\n' "$*" >&2
  status=1
}

warn() {
  printf 'warning: %s\n' "$*" >&2
}

check_command() {
  if command -v "$1" >/dev/null 2>&1; then
    ok "$1"
  else
    fail "$1"
  fi
}

check_executable() {
  if [[ -x "$1" ]]; then
    ok "$1"
  else
    fail "$1"
  fi
}

check_file() {
  if [[ -e "$1" ]]; then
    ok "$1"
  else
    fail "$1"
  fi
}

for command in big-red-button big-red-button-gui big-red-buttond ip wg nft resolvectl setpriv pkexec xdg-open; do
  check_command "${command}"
done
check_executable "${wstunnel}"
check_file /usr/share/applications/big-red-button.desktop
check_file /usr/share/icons/hicolor/scalable/apps/big-red-button.svg
check_file /usr/share/polkit-1/actions/com.myheartraces.bigredbutton.policy
check_file /usr/lib/systemd/system/big-red-buttond.service

if command -v systemctl >/dev/null 2>&1; then
  if systemctl is-active --quiet big-red-buttond.service; then
    ok "big-red-buttond.service active"
  else
    warn "big-red-buttond.service is not active"
  fi
fi

if command -v big-red-button >/dev/null 2>&1; then
  if big-red-button version >/dev/null 2>&1; then
    ok "big-red-button version"
  else
    fail "big-red-button version"
  fi
  if big-red-button daemon-status >/dev/null 2>&1; then
    ok "big-red-button daemon-status"
  else
    warn "big-red-button daemon-status failed; restart big-red-buttond.service"
  fi
fi

if [[ -n "${profile}" ]]; then
  if [[ ! -r "${profile}" ]]; then
    fail "profile is not readable: ${profile}"
  else
    big-red-button validate-profile "${profile}" || status=1
    big-red-button linux-preflight \
      -discover-routes \
      -require-pkexec \
      -wstunnel-binary "${wstunnel}" \
      "${profile}" || status=1
  fi
elif [[ "${install_check}" -eq 0 ]]; then
  echo
  echo "To validate a VPN profile:"
  echo "  big-red-button-check --profile /path/to/profile.json"
fi

exit "${status}"
