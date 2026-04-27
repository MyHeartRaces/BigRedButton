#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="${VERSION:-0.2.0}"
app_name="Big Red Button"
bundle_id="com.myheartraces.bigredbutton"
dist_dir="${repo_root}/dist/macos"
app_dir="${dist_dir}/${app_name}.app"
pkgroot="${dist_dir}/pkgroot"

rm -rf "${dist_dir}"
mkdir -p "${app_dir}/Contents/MacOS" "${app_dir}/Contents/Resources" "${pkgroot}/Applications"

(
  cd "${repo_root}"
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -buildvcs=false -ldflags "-s -w" -o "${app_dir}/Contents/Resources/big-red-button" ./cmd/big-red-button
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -buildvcs=false -ldflags "-s -w" -o "${app_dir}/Contents/MacOS/big-red-button-gui" ./cmd/big-red-button-gui
)

cat > "${app_dir}/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleDisplayName</key>
  <string>${app_name}</string>
  <key>CFBundleExecutable</key>
  <string>big-red-button-gui</string>
  <key>CFBundleIdentifier</key>
  <string>${bundle_id}</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>${app_name}</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>${version}</string>
  <key>CFBundleVersion</key>
  <string>${version}</string>
  <key>LSMinimumSystemVersion</key>
  <string>12.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
PLIST

cp -R "${app_dir}" "${pkgroot}/Applications/"
ditto -c -k --keepParent "${app_dir}" "${dist_dir}/big-red-button_darwin_arm64_app.zip"

if command -v pkgbuild >/dev/null 2>&1; then
  pkgbuild \
    --root "${pkgroot}" \
    --identifier "${bundle_id}" \
    --version "${version}" \
    --install-location "/" \
    "${dist_dir}/big-red-button_darwin_arm64.pkg"
else
  echo "pkgbuild was not found; skipping macOS .pkg creation" >&2
fi

echo
echo "macOS package output:"
find "${dist_dir}" -maxdepth 1 -type f -print
