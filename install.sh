#!/bin/bash
set -euo pipefail

OWNER="MedUnes"
REPO="dewdrops"
BINARY_NAME="dewdrops"
RELEASE_PATTERN="dewdrops_Linux_x86_64.tar.gz"
CHECKSUM_PATTERN="checksums.txt"

if ! command -v curl &> /dev/null; then
    echo "Error: curl is not installed. Please install it."
    exit 1
fi

if ! command -v jq &> /dev/null; then
    echo "Error: jq is not installed. Please install it (e.g., sudo apt-get install jq)."
    exit 1
fi

if ! command -v sha256sum &> /dev/null; then
    echo "Error: sha256sum is not installed. Please install it (usually part of coreutils)."
    exit 1
fi

echo "Fetching latest release information for $OWNER/$REPO..."
LATEST_RELEASE_INFO=$(curl -s "https://api.github.com/repos/$OWNER/$REPO/releases/latest")

if [ -z "$LATEST_RELEASE_INFO" ]; then
  echo "Error: Failed to fetch latest release information from GitHub API."
  exit 1
fi

RELEASE_URL=$(echo "$LATEST_RELEASE_INFO" | jq -r ".assets[] | select(.name | endswith(\"$RELEASE_PATTERN\")) | .browser_download_url")
CHECKSUM_URL=$(echo "$LATEST_RELEASE_INFO" | jq -r ".assets[] | select(.name | endswith(\"$CHECKSUM_PATTERN\")) | .browser_download_url")

if [ -z "$RELEASE_URL" ]; then
  echo "Error: Could not find the latest release asset matching the pattern '$RELEASE_PATTERN'."
  exit 1
fi

if [ -z "$CHECKSUM_URL" ]; then
  echo "Error: Could not find the latest release asset matching the pattern '$CHECKSUM_PATTERN'."
  exit 1
fi

RELEASE_FILE=$(basename "$RELEASE_URL")
CHECKSUM_FILE=$(basename "$CHECKSUM_URL")

echo "Found latest package: $RELEASE_FILE"
echo "Found latest checksum file: $CHECKSUM_FILE"

echo "Downloading package: $RELEASE_FILE"
if ! wget -q -O "$RELEASE_FILE" "$RELEASE_URL"; then
    echo "Error: Failed to download package."
    exit 1
fi

echo "Downloading checksum file: $CHECKSUM_FILE"
if ! wget -q -O "$CHECKSUM_FILE" "$CHECKSUM_URL"; then
    echo "Error: Failed to download checksum file."
    rm -f "$RELEASE_FILE"
    exit 1
fi
echo "Verifying checksum..."

EXPECTED_CHECKSUM=$(grep "$RELEASE_FILE" "$CHECKSUM_FILE" | awk '{print $1}')

if [ -z "$EXPECTED_CHECKSUM" ]; then
    echo "Error: Could not find checksum for '$RELEASE_FILE' in '$CHECKSUM_FILE'."
    rm -f "$RELEASE_FILE" "$CHECKSUM_FILE"
    exit 1
fi

ACTUAL_CHECKSUM=$(sha256sum "$RELEASE_FILE" | awk '{print $1}')

if [ "$ACTUAL_CHECKSUM" = "$EXPECTED_CHECKSUM" ]; then
    echo "Checksum verification successful."
else
    echo "Error: Checksum verification failed!"
    echo "  Expected: $EXPECTED_CHECKSUM"
    echo "  Actual:   $ACTUAL_CHECKSUM"
    rm -f "$RELEASE_FILE" "$CHECKSUM_FILE"
    exit 1
fi
echo "Unpacking tarball.."
tar -zxvf "$RELEASE_FILE" "$BINARY_NAME"
chmod +x "$BINARY_NAME"
echo "Cleaning up downloaded files."

rm "$CHECKSUM_FILE" "$RELEASE_FILE"

./$BINARY_NAME


exit 0

