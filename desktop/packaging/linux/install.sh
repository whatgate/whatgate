#!/usr/bin/env sh
set -eu

SOURCE_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
INSTALL_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/whatgate"
BIN_DIR="${HOME}/.local/bin"
APP_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/applications"

mkdir -p "$INSTALL_DIR" "$BIN_DIR" "$APP_DIR"
cp -R "$SOURCE_DIR/." "$INSTALL_DIR/"
chmod +x \
  "$INSTALL_DIR/WhatGate" \
  "$INSTALL_DIR/core/whatgate" \
  "$INSTALL_DIR/core/coordinator"
ln -sf "$INSTALL_DIR/WhatGate" "$BIN_DIR/whatgate-ui"

sed "s|__EXEC__|$INSTALL_DIR/WhatGate|g" \
  "$SOURCE_DIR/whatgate.desktop.in" > "$APP_DIR/whatgate.desktop"

echo "WhatGate 已安装。可从应用菜单启动，或运行 whatgate-ui。"
