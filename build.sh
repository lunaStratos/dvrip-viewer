#!/usr/bin/env bash
# 네이티브(Fyne) 앱 빌드 + macOS .app 번들(아이콘 포함) 생성
set -e
cd "$(dirname "$0")"
mkdir -p dist

echo "== 네이티브 앱 빌드 (Fyne/OpenGL, CGO) =="
go build -ldflags "-s -w" -o dist/xmview-native-mac-arm64 .

APP="dist/XMView-Native.app"
rm -rf "$APP"; mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp dist/xmview-native-mac-arm64 "$APP/Contents/MacOS/xmview"

echo "== 아이콘(.icns) 생성 =="
ICONSET=$(mktemp -d)/XMView.iconset; mkdir -p "$ICONSET"
for s in 16 32 64 128 256 512; do
  sips -z $s $s icon.png --out "$ICONSET/icon_${s}x${s}.png" >/dev/null
done
cp "$ICONSET/icon_32x32.png"   "$ICONSET/icon_16x16@2x.png"
cp "$ICONSET/icon_64x64.png"   "$ICONSET/icon_32x32@2x.png"
cp "$ICONSET/icon_256x256.png" "$ICONSET/icon_128x128@2x.png"
cp "$ICONSET/icon_512x512.png" "$ICONSET/icon_256x256@2x.png"
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/XMView.icns"

cat > "$APP/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>XMView</string>
  <key>CFBundleDisplayName</key><string>XMView</string>
  <key>CFBundleIdentifier</key><string>com.lunastratos.xmview.native</string>
  <key>CFBundleVersion</key><string>1.0</string>
  <key>CFBundleShortVersionString</key><string>1.0</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleExecutable</key><string>xmview</string>
  <key>CFBundleIconFile</key><string>XMView.icns</string>
  <key>LSMinimumSystemVersion</key><string>10.13</string>
  <key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST

# Finder 아이콘 캐시 갱신
touch "$APP"
echo "  → $APP (아이콘 적용됨)"
