#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
pkgname="big-red-button"
pkgver="${PKGVER:-0.2.1}"
workdir="${repo_root}/dist/arch/makepkg"
src_tar="${workdir}/${pkgname}-${pkgver}.tar.gz"

if ! command -v makepkg >/dev/null 2>&1; then
  echo "makepkg was not found. Run this on Arch Linux with pacman/devtools installed." >&2
  exit 1
fi

rm -rf "${workdir}"
mkdir -p "${workdir}"

git -C "${repo_root}" archive \
  --format=tar.gz \
  --prefix="${pkgname}-${pkgver}/" \
  -o "${src_tar}" \
  HEAD

cp "${repo_root}/packaging/arch/PKGBUILD" "${workdir}/PKGBUILD"

(
  cd "${workdir}"
  makepkg --syncdeps --clean --force --noconfirm
)

echo
echo "Arch package written to:"
find "${workdir}" -maxdepth 1 -type f -name "${pkgname}-${pkgver}-*.pkg.tar.*" -print
