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

# Where the web interface and this script talk to each other about the game
# servers: what each one SHOULD be doing (want-<n>: run or stop), what it is
# doing (state-<n>), and its process number (pid-<n>). It is rebuilt at every
# start, so the SERVER_<n>_AUTOSTART settings always decide the starting state.
CONTROL_DIR="${CONTROL_DIR:-/tmp/bfbc2-control}"
export CONTROL_DIR
rm -rf "$CONTROL_DIR"
mkdir -p "$CONTROL_DIR"

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

# Keeps one game server running (or stopped) as the "want" file says.
#   - want = run:  start it, and start it again 5 seconds after it stops
#   - want = stop: stop it and leave it stopped
# The web interface changes the want file.
supervise_server() {
    local n=$1
    local marker; marker=$(server_marker "$n")
    (
        local want pid i
        while [ ! -e "$STOPFLAG" ]; do
            want=$(cat "$CONTROL_DIR/want-${n}" 2> /dev/null || echo run)
            if [ "$want" = stop ]; then
                echo stopped > "$CONTROL_DIR/state-${n}"
                sleep 2
                continue
            fi
            echo running > "$CONTROL_DIR/state-${n}"
            run_server "$n" &
            pid=$!
            echo "$pid" > "$CONTROL_DIR/pid-${n}"

            # Found in testing: real Wine does not always keep running as the
            # very process we just started (it can hand off to another one
            # internally), so that pid is not reliably the one holding the
            # game's ports. Instead we find and watch the ACTUAL game process
            # by matching its unique command line ("$marker").
            i=0
            while ! pgrep -f "$marker" > /dev/null 2>&1; do
                i=$((i + 1))
                if [ "$i" -ge 100 ]; then break; fi        # 10 seconds: give up waiting
                kill -0 "$pid" 2> /dev/null || break        # it died before ever appearing
                sleep 0.1
            done

            while pgrep -f "$marker" > /dev/null 2>&1; do
                [ -e "$STOPFLAG" ] && break
                if [ "$(cat "$CONTROL_DIR/want-${n}" 2> /dev/null)" = stop ]; then
                    # SIGKILL, matched by command line (see the comment above).
                    # A plain SIGTERM only ends one thread of the game and
                    # leaves a half-dead server behind (found in testing).
                    pkill -KILL -f "$marker" 2> /dev/null || true
                    break
                fi
                sleep 1
            done

            # "|| true" matters here: under "set -e", wait's exit status is the
            # killed process's status (non-zero), which would otherwise end this
            # whole supervisor right here -- silently, with no restart ever again.
            wait "$pid" 2> /dev/null || true
            rm -f "$CONTROL_DIR/pid-${n}"
            [ -e "$STOPFLAG" ] && break
            if [ "$(cat "$CONTROL_DIR/want-${n}" 2> /dev/null)" != stop ]; then
                log "game server ${n} stopped. Starting it again in 5 seconds."
                sleep 5
            fi
        done
    ) &
}

run_master() {
    cd "$MASTERDIR"
    # stdin from /dev/null: on an error the master otherwise waits for ENTER.
    if [ "$MASTER_TAIL" = 1 ]; then
        # Its log file is shown in "docker logs" instead (see below), so its own
        # console output is dropped. Otherwise every line would appear twice.
        /opt/mase/mase_bc2 < /dev/null > /dev/null 2>&1
    else
        /opt/mase/mase_bc2 < /dev/null
    fi
}

# The exact text that identifies server <n>'s process on the command line,
# used to find and stop the right one (see the big comment in supervise_server).
server_marker() { printf 'serverInstancePath instances/%s/ ' "$1"; }

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
    exec wine Frost.Game.Main_Win32_Final.exe \
        -serverInstancePath "instances/${n}/" \
        -mapPack2Enabled "$mp2" \
        -timeStampLogNames \
        -region "$region" \
        -heartBeatInterval "$hb" \
        $extra
}

run_webui() {
    /opt/webui/bfbc2-webui
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
    pkill -TERM -x bfbc2-webui 2>/dev/null || true
    wineserver -k 2>/dev/null || true
    sleep 1
    [ -z "$XVFB_PID" ] || kill "$XVFB_PID" 2>/dev/null || true
    exit 0
}
trap shutdown TERM INT

if [ "$(norm_bool "${MASTER_ENABLED:-true}")" = true ]; then
    log "starting the master server"
    # The master's own console output is buffered and arrives late, but its log
    # file is complete. So we show that file in "docker logs"
    # (MASTER_LOG_TO_CONSOLE=false turns this off). It needs the plain file name,
    # so it is not used when the log file name carries a time stamp.
    MASTER_TAIL=0
    if [ "$(norm_bool "${MASTER_LOG_CREATE:-true}")" = true ] \
       && [ "$(norm_bool "${MASTER_LOG_TO_CONSOLE:-true}")" = true ] \
       && [ "$(norm_bool "${MASTER_LOG_TIMESTAMP:-false}")" = false ]; then
        MASTER_TAIL=1
    fi
    supervise "the master server" run_master
    if [ "$MASTER_TAIL" = 1 ]; then
        tail -n +1 -F "${MASTERDIR}/logfile.log" 2> /dev/null &
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
        if [ "$(norm_bool "$(setting "$n" AUTOSTART true)")" = true ]; then
            echo run > "$CONTROL_DIR/want-${n}"
            log "starting game server ${n}"
            supervise_server "$n"
            sleep 2    # the original launcher also waits between servers
        else
            echo stop > "$CONTROL_DIR/want-${n}"
            log "game server ${n} is set not to start by itself (SERVER_${n}_AUTOSTART). Start it from the web interface."
            supervise_server "$n"
        fi
    done
fi

# The web interface (needs at least one game server to manage).
if [ "$(norm_bool "${WEB_ENABLED:-true}")" = true ] && [ "${#SERVERS[@]}" -gt 0 ]; then
    log "starting the web interface on port ${WEB_PORT:-5010}"
    supervise "the web interface" run_webui
fi

log "everything is started"
# Wait here. A stop signal runs the shutdown function above.
wait
