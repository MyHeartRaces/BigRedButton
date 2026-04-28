#!/usr/bin/env bash
set -euo pipefail

profile=""
wstunnel_binary=""
endpoint_ip=""
runtime_root="/run/big-red-button"
bundle=""
real_connect=0
real_isolated=0
skip_isolated=0
isolated_session_id=""
isolated_command=("/usr/bin/true")

usage() {
  cat <<'USAGE'
usage: scripts/linux-smoke.sh --profile profile.json [options]

Options:
  --profile path             VPN profile JSON. Required.
  --wstunnel-binary path     WSTunnel binary path/name.
  --endpoint-ip ip[,ip]      Optional resolved tunnel gateway IP override.
  --runtime-root path        Runtime root. Defaults to /run/big-red-button.
  --bundle path.tar.gz       Diagnostics bundle output path.
  --real-connect             Also run privileged connect/status/disconnect.
  --skip-isolated            Skip isolated app tunnel dry-run/preflight checks.
  --isolated-session uuid    Session UUID for isolated app checks.
  --isolated-command path    Isolated app command. Defaults to /usr/bin/true.
  --isolated-arg value       Append an argument to the isolated app command.
  --real-isolated            Also run privileged isolated app start/recovery.
  -h, --help                 Show this help.

By default this script runs non-mutating validation, Linux preflight, system
dry-run, isolated app dry-run/preflight and a redacted diagnostics bundle.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile)
      profile="${2:-}"
      shift 2
      ;;
    --wstunnel-binary)
      wstunnel_binary="${2:-}"
      shift 2
      ;;
    --endpoint-ip)
      endpoint_ip="${2:-}"
      shift 2
      ;;
    --runtime-root)
      runtime_root="${2:-}"
      shift 2
      ;;
    --bundle)
      bundle="${2:-}"
      shift 2
      ;;
    --real-connect)
      real_connect=1
      shift
      ;;
    --skip-isolated)
      skip_isolated=1
      shift
      ;;
    --isolated-session)
      isolated_session_id="${2:-}"
      shift 2
      ;;
    --isolated-command)
      isolated_command=("${2:-}")
      shift 2
      ;;
    --isolated-arg)
      isolated_command+=("${2:-}")
      shift 2
      ;;
    --real-isolated)
      real_isolated=1
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

if [[ -z "${profile}" ]]; then
  echo "--profile is required" >&2
  usage >&2
  exit 2
fi

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "linux-smoke.sh can only run on Linux" >&2
  exit 1
fi

if [[ -z "${BRB_CLI:-}" ]]; then
  if [[ -x "./build/big-red-button" ]]; then
    BRB_CLI="./build/big-red-button"
  else
    BRB_CLI="big-red-button"
  fi
fi

if ! command -v "${BRB_CLI}" >/dev/null 2>&1 && [[ ! -x "${BRB_CLI}" ]]; then
  echo "Big Red Button CLI was not found: ${BRB_CLI}" >&2
  echo "Set BRB_CLI=/path/to/big-red-button or install the package." >&2
  exit 1
fi

if [[ -z "${bundle}" ]]; then
  bundle="big-red-button-diagnostics-$(date -u +%Y%m%d-%H%M%S).tar.gz"
fi

generate_uuid() {
  if command -v uuidgen >/dev/null 2>&1; then
    uuidgen | tr 'A-F' 'a-f'
    return
  fi
  if [[ -r /proc/sys/kernel/random/uuid ]]; then
    cat /proc/sys/kernel/random/uuid
    return
  fi
  echo "unable to generate isolated session UUID" >&2
  return 1
}

if [[ -z "${isolated_session_id}" ]]; then
  isolated_session_id="$(generate_uuid)"
fi

profile_args=("${profile}")
common_args=("-runtime-root" "${runtime_root}")
connect_args=("-runtime-root" "${runtime_root}")
preflight_args=("-discover-routes" "-require-pkexec")
dry_run_args=()
diagnostics_args=("-runtime-root" "${runtime_root}")
bundle_args=("-runtime-root" "${runtime_root}" "-output" "${bundle}")
isolated_args=("-runtime-root" "${runtime_root}" "-session-id" "${isolated_session_id}")
isolated_preflight_args=("-require-pkexec" "${isolated_args[@]}")

if [[ -n "${wstunnel_binary}" ]]; then
  connect_args+=("-wstunnel-binary" "${wstunnel_binary}")
  preflight_args+=("-wstunnel-binary" "${wstunnel_binary}")
  dry_run_args+=("-wstunnel-binary" "${wstunnel_binary}")
  diagnostics_args+=("-wstunnel-binary" "${wstunnel_binary}")
  bundle_args+=("-wstunnel-binary" "${wstunnel_binary}")
  isolated_args+=("-wstunnel-binary" "${wstunnel_binary}")
  isolated_preflight_args+=("-wstunnel-binary" "${wstunnel_binary}")
fi

if [[ -n "${endpoint_ip}" ]]; then
  connect_args+=("-endpoint-ip" "${endpoint_ip}")
  preflight_args+=("-endpoint-ip" "${endpoint_ip}")
  dry_run_args+=("-endpoint-ip" "${endpoint_ip}")
fi

run_step() {
  echo
  echo "==> $*"
  "$@"
}

run_privileged() {
  if [[ "${EUID}" -eq 0 ]]; then
    "$@"
  else
    sudo "$@"
  fi
}

cleanup_real_connect() {
  if [[ "${real_connect}" -eq 1 ]]; then
    run_privileged "${BRB_CLI}" linux-disconnect -yes "${common_args[@]}" >/dev/null 2>&1 || true
  fi
  if [[ "${real_isolated}" -eq 1 ]]; then
    run_privileged "${BRB_CLI}" linux-cleanup-isolated-app -yes "${common_args[@]}" -session-id "${isolated_session_id}" >/dev/null 2>&1 || true
  fi
}
trap cleanup_real_connect EXIT

run_step "${BRB_CLI}" version
run_step "${BRB_CLI}" validate-profile "${profile_args[@]}"
run_step "${BRB_CLI}" linux-preflight "${preflight_args[@]}" "${profile_args[@]}"
run_step "${BRB_CLI}" linux-dry-run-connect "${dry_run_args[@]}" "${profile_args[@]}"
if [[ "${skip_isolated}" -eq 0 ]]; then
  run_step "${BRB_CLI}" plan-isolated-app "${isolated_args[@]}" "${profile}" -- "${isolated_command[@]}"
  run_step "${BRB_CLI}" linux-dry-run-isolated-app "${isolated_args[@]}" "${profile}" -- "${isolated_command[@]}"
  run_step "${BRB_CLI}" linux-preflight-isolated-app "${isolated_preflight_args[@]}" "${profile}" -- "${isolated_command[@]}"
fi
run_step "${BRB_CLI}" diagnostics "${diagnostics_args[@]}" -profile "${profile}"
run_step "${BRB_CLI}" diagnostics-bundle "${bundle_args[@]}" -profile "${profile}"

if [[ "${real_connect}" -eq 1 ]]; then
  echo
  echo "==> privileged connect/status/disconnect"
  run_privileged "${BRB_CLI}" linux-connect -yes "${connect_args[@]}" "${profile}"
  run_privileged "${BRB_CLI}" status "${common_args[@]}"
  run_privileged "${BRB_CLI}" linux-disconnect -yes "${common_args[@]}"
  real_connect=0
fi

if [[ "${real_isolated}" -eq 1 ]]; then
  echo
  echo "==> privileged isolated app start/recovery"
  run_privileged "${BRB_CLI}" linux-isolated-app -yes "${isolated_args[@]}" "${profile}" -- "${isolated_command[@]}"
  sleep 2
  run_privileged "${BRB_CLI}" linux-recover-isolated-sessions -yes -startup "${common_args[@]}"
  "${BRB_CLI}" isolated-sessions "${common_args[@]}"
  real_isolated=0
fi

echo
echo "Smoke run completed."
echo "Diagnostics bundle: ${bundle}"
