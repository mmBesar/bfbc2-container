#!/bin/bash
# =============================================================================
# fetch-pack - get the BFBC2 server pack ready, once.
#
# In plain words:
#   1. If the pack is already unpacked (and is the right version), do nothing.
#   2. Otherwise get the .rar (download it, or use your own copy),
#      check its SHA256, and unpack it into PACK_DIR.
#
# Settings (environment variables):
#   PACK_SHA256  required. Expected SHA256 of Bc2emu_V09.rar
#   PACK_URL     where to download from (default: the SourceForge project)
#   PACK_DIR     where to unpack to      (default: /data/pack)
#   PACK_FILE    optional. Path to your OWN copy of the .rar. If set, nothing
#                is downloaded. Good for keeping a backup copy on your NAS.
# =============================================================================
set -euo pipefail

PACK_URL="${PACK_URL:-https://downloads.sourceforge.net/project/battlefieldbadcompany2mase/Bc2emu_V09.rar}"
PACK_SHA256="${PACK_SHA256:?PACK_SHA256 must be set}"
PACK_DIR="${PACK_DIR:-/data/pack}"
PACK_FILE="${PACK_FILE:-}"

# A small file we write after a successful unpack, so next start can skip it.
MARKER="${PACK_DIR}/.pack-sha256"

# 1. Already done with the right version? Then we are finished.
if [ -f "${MARKER}" ] && [ "$(cat "${MARKER}")" = "${PACK_SHA256}" ]; then
    echo "fetch-pack: pack already present (${PACK_SHA256}), nothing to do"
    exit 0
fi

# Unpack into a temporary folder first, so a half-finished job never
# leaves a broken PACK_DIR behind.
WORK="${PACK_DIR}.tmp"
rm -rf "${WORK}"
mkdir -p "${WORK}"

# 2. Get the .rar
if [ -n "${PACK_FILE}" ]; then
    echo "fetch-pack: using local file ${PACK_FILE}"
    RAR="${PACK_FILE}"
else
    echo "fetch-pack: downloading ${PACK_URL}"
    RAR="${WORK}/pack.rar"
    wget -nv --tries=3 -O "${RAR}" "${PACK_URL}"
fi

# 3. Verify. If this does not match, the script stops here (set -e).
echo "${PACK_SHA256}  ${RAR}" | sha256sum -c -

# 4. Unpack. The archive has one top-level folder called "Bc2emu".
#    We skip the source-code zip and old log / crash files we do not need.
echo "fetch-pack: unpacking"
unrar x -idq -y "${RAR}" \
    -x'Bc2emu/Source and Stuff' \
    -x'*.log' \
    -x'*crashreport.xml' \
    "${WORK}/"

# 5. Move into place and write the marker.
rm -rf "${PACK_DIR}"
mv "${WORK}/Bc2emu" "${PACK_DIR}"
rm -rf "${WORK}"
echo "${PACK_SHA256}" > "${MARKER}"

echo "fetch-pack: done, pack is in ${PACK_DIR}"
