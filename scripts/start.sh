#!/bin/bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 mmBesar
# =============================================================================
# start.sh - runs as the unprivileged user (see entrypoint.sh).
#
# In plain words:
#   1. Get the server pack ready (download once, verify, unpack).
#   2. Turn the environment variables into config files.
#   3. Start the master server, one shared fake screen, and every game server.
#   4. If one of them stops, start it again. On shutdown, stop everything.
#
# For testing without Wine, set DRY_RUN=1: it only generates the config files.
# =============================================================================
set -euo pipefail

# Where things live. All of it is inside /data, the one folder you mount.
: "${DATA_DIR:=/data}"
: "${PACK_DIR:=${DATA_DIR}/pack}"
: "${INSTANCE_ROOT:=${DATA_DIR}/instances}"
: "${CONFIG_DIR:=${DATA_DIR}/config}"
: "${MASTERDIR:=${DATA_DIR}/master}"
: "${LIB_DIR:=/usr/local/lib/bfbc2}"
export DATA_DIR PACK_DIR INSTANCE_ROOT CONFIG_DIR MASTERDIR

# shellcheck source=lib.sh
source "${LIB_DIR}/lib.sh"

# ---- 1. the server pack -------------------------------------------------------
if [ "${DRY_RUN:-0}" != 1 ]; then
    fetch-pack
fi

# ---- 2. config files ----------------------------------------------------------
generate_all

if [ "${DRY_RUN:-0}" = 1 ]; then
    log "dry run finished"
    exit 0
fi

# ---- 3. start everything --------------------------------------------------------
STOPFLAG=/tmp/aio-stopping
rm -f "$STOPFLAG"

# Run a command; if it stops, start it again after 5 seconds.
supervise() {
    local name=$1; shift
    (
        while [ ! -e "$STOPFLAG" ]; do
            "$@" || true
            [ -e "$STOPFLAG" ] && break
            log "${name} stopped. Starting it again in 5 seconds."
            sleep 5
        done
    ) &
}

run_master() {
    cd "$MASTERDIR"
    # stdin from /dev/null: on an error the master otherwise waits for ENTER.
    /opt/mase/mase_bc2 < /dev/null
}

run_server() {
    local n=$1
    local region hb mp2 extra
    region=$(setting "$n" REGION OC)
    hb=$(setting "$n" HEARTBEAT 20000)
    mp2=$(setting "$n" MAPPACK2 1)
    extra=$(setting "$n" EXTRA_ARGS "")
    cd "$PACK_DIR"
    # $extra is left unquoted on purpose: it may hold several arguments.
    # shellcheck disable=SC2086
    wine Frost.Game.Main_Win32_Final.exe \
        -serverInstancePath "instances/${n}/" \
        -mapPack2Enabled "$mp2" \
        -timeStampLogNames \
        -region "$region" \
        -heartBeatInterval "$hb" \
        $extra
}

# Stop everything in a sensible order: first the restart loops and the master,
# then Wine (all game servers), and only then the fake screen. Stopping the
# screen first makes Wine print "X connection broken" errors.
XVFB_PID=""
shutdown() {
    log "shutting down"
    touch "$STOPFLAG"
    local pid
    for pid in $(jobs -p); do
        [ "$pid" = "$XVFB_PID" ] || kill "$pid" 2>/dev/null || true
    done
    pkill -TERM -x mase_bc2 2>/dev/null || true
    wineserver -k 2>/dev/null || true
    sleep 1
    [ -z "$XVFB_PID" ] || kill "$XVFB_PID" 2>/dev/null || true
    exit 0
}
trap shutdown TERM INT

if [ "$(norm_bool "${MASTER_ENABLED:-true}")" = true ]; then
    log "starting the master server"
    supervise "the master server" run_master
    # The master's own console output is buffered and arrives late, but its log
    # file is complete. Show that file in "docker logs" (set MASTER_LOG_TO_CONSOLE=false to stop).
    if [ "$(norm_bool "${MASTER_LOG_CREATE:-true}")" = true ] && [ "$(norm_bool "${MASTER_LOG_TO_CONSOLE:-true}")" = true ]; then
        tail -n 0 -F "${MASTERDIR}/logfile.log" 2> /dev/null &
    fi
    sleep 3    # the original launcher also gives the master a head start
else
    log "MASTER_ENABLED is false: not starting a master. Servers will look for one at ${MASTER_HOST:-127.0.0.1}"
fi

if [ "${#SERVERS[@]}" -gt 0 ]; then
    # One fake screen for all servers. Started directly (not with xvfb-run).
    export DISPLAY=:99
    Xvfb :99 -nolisten tcp -screen 0 1280x1024x24 &
    XVFB_PID=$!
    for _ in $(seq 1 50); do
        [ -S /tmp/.X11-unix/X99 ] && break
        sleep 0.2
    done

    for n in "${SERVERS[@]}"; do
        log "starting game server ${n}"
        supervise "game server ${n}" run_server "$n"
        sleep 2    # the original launcher also waits between servers
    done
fi

log "everything is started"
# Wait here. A stop signal runs the shutdown function above.
wait
