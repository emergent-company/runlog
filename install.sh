#!/bin/sh
# install.sh — download and install the latest runlog binary.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/emergent-company/runlog/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/emergent-company/runlog/main/install.sh | sh -s -- --dir /usr/local/bin
#   curl -fsSL https://raw.githubusercontent.com/emergent-company/runlog/main/install.sh | sh -s -- --version v0.1.0

set -e

REPO="emergent-company/runlog"
INSTALL_DIR="${HOME}/.local/bin"
VERSION=""

# Parse arguments.
while [ $# -gt 0 ]; do
    case "$1" in
        --dir)     INSTALL_DIR="$2"; shift 2 ;;
        --version) VERSION="$2"; shift 2 ;;
        *)         echo "unknown option: $1" >&2; exit 1 ;;
    esac
done

# Detect OS and architecture.
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
    *)       echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

case "$OS" in
    linux)  ;;
    darwin) ;;
    *)      echo "unsupported OS: $OS" >&2; exit 1 ;;
esac

# Resolve version (latest release if not specified).
if [ -z "$VERSION" ]; then
    VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')"
    if [ -z "$VERSION" ]; then
        echo "error: could not determine latest version" >&2
        exit 1
    fi
fi

# Strip leading "v" for the archive name.
VERSION_NUM="${VERSION#v}"

# Download and extract.
ARCHIVE="runlog_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

echo "Downloading runlog ${VERSION} for ${OS}/${ARCH}..."
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

curl -fsSL "$URL" -o "${TMPDIR}/${ARCHIVE}"
tar -xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"

# Install binary.
mkdir -p "$INSTALL_DIR"
mv "${TMPDIR}/runlog" "${INSTALL_DIR}/runlog"
chmod +x "${INSTALL_DIR}/runlog"

echo "Installed runlog ${VERSION} to ${INSTALL_DIR}/runlog"

# Check if INSTALL_DIR is in PATH.
case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *)
        echo ""
        echo "Note: ${INSTALL_DIR} is not in your PATH."
        echo "Add it with:  export PATH=\"${INSTALL_DIR}:\$PATH\""
        ;;
esac
