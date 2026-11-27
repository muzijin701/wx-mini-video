#!/usr/bin/env sh
set -eu

REPO="${WX_MINI_VIDEO_REPO:-muzijin701/wx-mini-video}"
INSTALL_DIR="${WX_MINI_VIDEO_INSTALL_DIR:-$HOME/.local/share/wx-mini-video}"
BIN_DIR="${WX_MINI_VIDEO_BIN_DIR:-$HOME/.local/bin}"
LOCAL_ARCHIVE="${WX_MINI_VIDEO_ARCHIVE:-}"
NO_MODIFY_PATH="${WX_MINI_VIDEO_NO_MODIFY_PATH:-0}"

OS_RAW="$(uname -s)"
if [ "$OS_RAW" != "Darwin" ]; then
  echo "Error: wx-mini-video only publishes releases for Windows and macOS (detected '$OS_RAW')." >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

if [ -n "$LOCAL_ARCHIVE" ]; then
  ARCHIVE_PATH="$LOCAL_ARCHIVE"
else
  API_URL="https://api.github.com/repos/$REPO/releases/latest"
  echo "Fetching latest release metadata from $REPO..."
  ASSET_URL="$(curl -fsSL "$API_URL" | grep -Eo 'https://[^"]+wx-mini-video-macos-universal-unsigned\.zip' | head -n 1 || true)"
  if [ -z "$ASSET_URL" ]; then
    echo "Error: Could not find macOS release asset in $REPO." >&2
    exit 1
  fi
  ARCHIVE_PATH="$TMP_DIR/wx-mini-video-macos.zip"
  echo "Downloading $ASSET_URL..."
  curl -fsSL "$ASSET_URL" -o "$ARCHIVE_PATH"
fi

BACKUP_DIR="$TMP_DIR/backup"
EXTRACT_DIR="$TMP_DIR/extract"
mkdir -p "$BACKUP_DIR" "$EXTRACT_DIR"

if [ -d "$INSTALL_DIR" ]; then
  for item in wx-mini-video.yaml downloads; do
    if [ -e "$INSTALL_DIR/$item" ]; then
      mv "$INSTALL_DIR/$item" "$BACKUP_DIR/$item"
    fi
  done
  rm -rf "$INSTALL_DIR"
fi

unzip -q "$ARCHIVE_PATH" -d "$EXTRACT_DIR"
if [ -d "$EXTRACT_DIR/wx-mini-video-macos" ]; then
  SRC_DIR="$EXTRACT_DIR/wx-mini-video-macos"
else
  SRC_DIR="$EXTRACT_DIR"
fi

mkdir -p "$INSTALL_DIR" "$BIN_DIR"
cp -R "$SRC_DIR"/. "$INSTALL_DIR"/

chmod +x "$INSTALL_DIR/wx-mini-video.app/Contents/MacOS/wx-mini-video"
if [ -d "$INSTALL_DIR/ffmpeg" ]; then
  chmod +x "$INSTALL_DIR"/ffmpeg/ffmpeg-* 2>/dev/null || true
fi

for item in wx-mini-video.yaml downloads; do
  if [ -e "$BACKUP_DIR/$item" ]; then
    rm -rf "$INSTALL_DIR/$item"
    mv "$BACKUP_DIR/$item" "$INSTALL_DIR/$item"
  fi
done

ln -sf "$INSTALL_DIR/wx-mini-video.app/Contents/MacOS/wx-mini-video" "$BIN_DIR/wx-mini-video"

if [ "$NO_MODIFY_PATH" != "1" ]; then
  case ":${PATH}:" in
    *":${BIN_DIR}:"*) ;;
    *)
      echo "Note: Add '$BIN_DIR' to your PATH (e.g., export PATH=\"$BIN_DIR:\$PATH\")."
      ;;
  esac
fi

echo "Installed wx-mini-video (macOS unsigned preview) successfully!"
echo "Run: $BIN_DIR/wx-mini-video (or open \"$INSTALL_DIR/wx-mini-video.app\")"
