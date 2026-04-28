#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
pkgname="big-red-button"
pkgver="${PKGVER:-0.2.1}"
workdir="${repo_root}/dist/arch/makepkg"
src_tar="${workdir}/${pkgname}-${pkgver}.tar.gz"
tmp_root=""

cleanup() {
  if [[ -n "${tmp_root}" ]]; then
    rm -rf "${tmp_root}"
  fi
}
trap cleanup EXIT

if ! command -v makepkg >/dev/null 2>&1; then
  echo "makepkg was not found. Run this on Arch Linux with pacman/devtools installed." >&2
  exit 1
fi

rm -rf "${workdir}"
mkdir -p "${workdir}"

tmp_root="$(mktemp -d)"
src_root="${tmp_root}/${pkgname}-${pkgver}"
mkdir -p "${src_root}"
cp -R "${repo_root}/." "${src_root}/"
rm -rf \
  "${src_root}/.git" \
  "${src_root}/build" \
  "${src_root}/dist"
find "${src_root}" -name ".DS_Store" -delete
tar -czf "${src_tar}" -C "${tmp_root}" "${pkgname}-${pkgver}"

cp "${repo_root}/packaging/arch/PKGBUILD" "${workdir}/PKGBUILD"
sed -i "s/^pkgver=.*/pkgver=${pkgver}/" "${workdir}/PKGBUILD"

(
  cd "${workdir}"
  makepkg --syncdeps --clean --force --noconfirm
)

echo
echo "Arch package written to:"
find "${workdir}" -maxdepth 1 -type f -name "${pkgname}-${pkgver}-*.pkg.tar.*" -print
