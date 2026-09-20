#!/bin/bash
# =============================================================================
# entrypoint.sh - runs first, as root.
#
# In plain words:
#   1. Set the time zone from TZ.
#   2. Make the internal "bfbc2" user use the user/group IDs you chose with
#      PUID and PGID, so files in /data belong to you on the host.
#   3. Hand over to start.sh as that user.
#
# If the container was started with a fixed user (docker run --user / compose
# "user:"), none of the above is possible, so it just runs start.sh.
# =============================================================================
set -euo pipefail

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"
DATA_DIR="${DATA_DIR:-/data}"

# Clean up leftovers from an earlier run. /tmp survives "docker restart", and
# old Wine or fake-screen files (owned by another user ID, or left behind by a
# killed run) would block the new start. Nothing is running yet, so this is safe.
rm -rf /tmp/wine-* /tmp/.wine-* /tmp/.X99-lock /tmp/.X11-unix/X99 /tmp/aio-stopping 2>/dev/null || true

if [ "$(id -u)" != 0 ]; then
    echo "[aio] running as user $(id -u):$(id -g); PUID/PGID/TZ are ignored"
    exec /usr/local/bin/start.sh "$@"
fi

# ---- 1. time zone ----
if [ -n "${TZ:-}" ]; then
    if [ -f "/usr/share/zoneinfo/${TZ}" ]; then
        ln -sf "/usr/share/zoneinfo/${TZ}" /etc/localtime
        echo "${TZ}" > /etc/timezone
    else
        echo "[aio] WARNING: TZ '${TZ}' is not a known time zone, keeping UTC" >&2
    fi
fi

# ---- 2. user and group IDs ----
if [ "$(id -g bfbc2)" != "${PGID}" ]; then
    groupmod -o -g "${PGID}" bfbc2
fi
if [ "$(id -u bfbc2)" != "${PUID}" ]; then
    # This also hands the user's home folder (the Wine setup) to the new ID.
    usermod -o -u "${PUID}" bfbc2
fi

mkdir -p "${DATA_DIR}"
# Only re-own /data when it belongs to someone else (it can be large).
if [ "$(stat -c %u "${DATA_DIR}")" != "${PUID}" ] || [ "$(stat -c %g "${DATA_DIR}")" != "${PGID}" ]; then
    echo "[aio] setting owner of ${DATA_DIR} to ${PUID}:${PGID}"
    chown -R "${PUID}:${PGID}" "${DATA_DIR}"
fi

# The fake screen needs this folder to belong to root with the sticky bit.
mkdir -p /tmp/.X11-unix
chmod 1777 /tmp/.X11-unix

# ---- 3. drop privileges and start ----
export HOME=/home/bfbc2
echo "[aio] starting as bfbc2 (PUID=${PUID}, PGID=${PGID}, TZ=${TZ:-UTC})"
exec gosu bfbc2 /usr/local/bin/start.sh "$@"
