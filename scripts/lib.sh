#!/bin/bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 mmBesar
# =============================================================================
# lib.sh - turns Docker Compose environment variables into config files.
#
# Everything the container needs is controlled by environment variables, so
# nothing has to be edited by hand. This file only contains functions. It is
# used by start.sh.
#
# Naming scheme (details in docker-compose.example.yml):
#   MASTER_<KEY>              any setting of the master server's config.ini
#                             (MASTER_ALL_STATS_UNLOCKED=true -> all_stats_unlocked = true)
#   HOOK_<KEY>                a few gameplay options shared by all servers
#   SERVER_<KEY>              default for EVERY game server
#   SERVER_<n>_<KEY>          setting for game server number n (wins over SERVER_<KEY>)
#   SERVER_<n>_OPT_<Key>      any key of that server's ServerOptions.ini
#   SERVER_<n>_VAR_<name>     any "vars.<name>" startup setting of that server
#   SERVER_<n>_STARTUP        extra raw startup lines for that server
# A game server exists when SERVER_<n>_TYPE is set.
# =============================================================================

log()  { echo "[aio] $*"; }
warn() { echo "[aio] WARNING: $*" >&2; }

# ---- small helpers ----------------------------------------------------------

# Turn true/1/yes/on and false/0/no/off into "true" or "false".
norm_bool() {
    case "${1,,}" in
        true|1|yes|on)  echo true ;;
        false|0|no|off) echo false ;;
        *) warn "'$1' is not a valid true/false value, using false"; echo false ;;
    esac
}

# setting <n> <KEY> <default>
# Value for server n. Order: SERVER_<n>_<KEY>, then SERVER_<KEY>, then default.
# An explicitly EMPTY variable counts as set (so it can clear a global default).
setting() {
    local n=$1 key=$2 default=${3-}
    local per="SERVER_${n}_${key}" glob="SERVER_${key}"
    if   [ -n "${!per+x}" ];  then printf '%s' "${!per}"
    elif [ -n "${!glob+x}" ]; then printf '%s' "${!glob}"
    else printf '%s' "${default}"; fi
}

# own <n> <KEY> <default>
# Like setting, but only SERVER_<n>_<KEY> counts. Used for things that must be
# different for every server (type, ports, map list).
own() {
    local n=$1 key=$2 default=${3-}
    local per="SERVER_${n}_${key}"
    if [ -n "${!per+x}" ]; then printf '%s' "${!per}"; else printf '%s' "${default}"; fi
}

# Numbers of all defined servers (those with SERVER_<n>_TYPE), sorted.
list_servers() {
    compgen -A variable | sed -n 's/^SERVER_\([0-9][0-9]*\)_TYPE$/\1/p' | sort -n -u
}

# Ordered key/value collections used to build the .ini and startup files.
# (Setting an existing key again replaces its value and keeps its position.)
opt_reset() { unset OPT_VAL OPT_ORDER; declare -gA OPT_VAL=(); declare -ga OPT_ORDER=(); }
opt_set()   { if [ -z "${OPT_VAL[$1]+x}" ]; then OPT_ORDER+=("$1"); fi; OPT_VAL[$1]=$2; }
var_reset() { unset VAR_VAL VAR_ORDER; declare -gA VAR_VAL=(); declare -ga VAR_ORDER=(); }
var_set()   { if [ -z "${VAR_VAL[$1]+x}" ]; then VAR_ORDER+=("$1"); fi; VAR_VAL[$1]=$2; }

# set_ini_value <file> <key> <value>
# Replace the value of "key = value  ; comment" and keep the comment.
# Returns 1 if the key does not exist in the file.
set_ini_value() {
    local file=$1 key=$2 val=$3 tmp
    tmp=$(mktemp)
    if awk -v k="$key" -v v="$val" '
        { if (!done && match($0, "^" k "[ \t]*=")) {
              c = index($0, ";")
              rest = (c > 0) ? "\t" substr($0, c) : ""
              print k " = " v rest
              done = 1
          } else print }
        END { exit (done ? 0 : 3) }' "$file" > "$tmp"; then
        chmod 644 "$tmp"; mv "$tmp" "$file"
    else
        rm -f "$tmp"; return 1
    fi
}

# ---- game modes --------------------------------------------------------------

# mode_info <type>
# Sets MODE_GAMEMOD (BC2 or VIETNAM), MODE_PLAYLIST (first line of a map list),
# MODE_PACKLIST (the pack's own map list file, if any) and MODE_LEVEL_GLOB
# (how to find this mode's maps in the pack's level folders).
mode_info() {
    MODE_PACKLIST=""; MODE_LEVEL_GLOB=""
    case "${1,,}" in
        rush)          MODE_GAMEMOD=BC2;     MODE_PLAYLIST=RUSH;     MODE_PACKLIST=maplist_rush.txt ;;
        conq|conquest) MODE_GAMEMOD=BC2;     MODE_PLAYLIST=CONQUEST; MODE_PACKLIST=maplist_conquest.txt ;;
        sqdm)          MODE_GAMEMOD=BC2;     MODE_PLAYLIST=SQDM;     MODE_PACKLIST=maplist_sqdm.txt ;;
        sqrush)        MODE_GAMEMOD=BC2;     MODE_PLAYLIST=SQRUSH;   MODE_PACKLIST=maplist_sqrush.txt ;;
        vietrush)      MODE_GAMEMOD=VIETNAM; MODE_PLAYLIST=RUSH;     MODE_LEVEL_GLOB='nam_mp_[0-9][0-9][0-9]r' ;;
        vietconq)      MODE_GAMEMOD=VIETNAM; MODE_PLAYLIST=CONQUEST; MODE_LEVEL_GLOB='nam_mp_[0-9][0-9][0-9]cq' ;;
        vietsqdm)      MODE_GAMEMOD=VIETNAM; MODE_PLAYLIST=SQDM;     MODE_LEVEL_GLOB='nam_mp_[0-9][0-9][0-9]sdm' ;;
        vietsqrush)    MODE_GAMEMOD=VIETNAM; MODE_PLAYLIST=SQRUSH;   MODE_LEVEL_GLOB='nam_mp_[0-9][0-9][0-9]sr' ;;
        *) return 1 ;;
    esac
}

# ---- generators ---------------------------------------------------------------

# One RCON password for servers that do not set their own. If none is given,
# make a random one once and keep it in the config folder.
gen_rcon_default() {
    if [ -n "${SERVER_RCON_PASSWORD+x}" ]; then
        RCON_DEFAULT_PW=$SERVER_RCON_PASSWORD
        return 0
    fi
    local file="${CONFIG_DIR}/rcon-password"
    if [ ! -s "$file" ]; then
        LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 16 > "$file" || true
        chmod 600 "$file"
        log "SERVER_RCON_PASSWORD is not set. Generated a random one and saved it in ${file}"
    fi
    RCON_DEFAULT_PW=$(cat "$file")
}

# The client-side "hook" settings file next to the game executable.
# It is shared by all servers in this container.
gen_hook() {
    cat > "${PACK_DIR}/bfbc2.ini" <<EOF
[info]
host=${MASTER_HOST:-127.0.0.1}
connect_to_retail=0
executable_type=auto
show_console=0

[client]
reroute_http=0

[server]
ranking_min_players=${HOOK_RANKING_MIN_PLAYERS:-1}
deserting_allowed=${HOOK_DESERTING_ALLOWED:-0}
instant_spawn=${HOOK_INSTANT_SPAWN:-0}
unlimited_ammo=${HOOK_UNLIMITED_AMMO:-0}
health_mode=${HOOK_HEALTH_MODE:-0}
EOF
}

# The master server's folder: config.ini (from the pack's own file plus every
# MASTER_<KEY> variable), templates/ and database/ (kept between restarts).
gen_master_config() {
    mkdir -p "${MASTERDIR}/database"
    if [ ! -d "${MASTERDIR}/templates" ]; then
        cp -r "${PACK_DIR}/MasterServerEmu/templates" "${MASTERDIR}/templates"
    fi
    local f
    for f in users.lst personas.lst; do
        [ -e "${MASTERDIR}/database/${f}" ] || : > "${MASTERDIR}/database/${f}"
    done

    cp "${PACK_DIR}/MasterServerEmu/config.ini" "${MASTERDIR}/config.ini"
    chmod 644 "${MASTERDIR}/config.ini"

    # Clients on other machines need to reach the master, so listen everywhere
    # unless told otherwise.
    : "${MASTER_EMULATOR_IP:=0.0.0.0}"

    local v key
    for v in $(compgen -A variable MASTER_ | sort); do
        case "$v" in
            MASTER_ENABLED|MASTER_HOST) continue ;;   # our own settings, not config.ini keys
        esac
        key=${v#MASTER_}; key=${key,,}
        if ! set_ini_value "${MASTERDIR}/config.ini" "$key" "${!v}"; then
            warn "${v} ignored: config.ini has no option called '${key}'"
        fi
    done
}

# Map list for one server (MAPLIST variable > pack's own list > auto-detected).
gen_maplist() {
    local n=$1 inst=$2 list out="${2}/maplist.txt"
    list=$(own "$n" MAPLIST "")
    if [ -n "$list" ]; then
        {
            echo "$MODE_PLAYLIST"
            printf '%s\n' "$list" | tr ',' '\n' \
                | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' -e '/^$/d' -e '/\//! s|^|levels/|'
        } > "$out"
    elif [ -n "$MODE_PACKLIST" ] && [ -f "${PACK_DIR}/Instance/${MODE_PACKLIST}" ]; then
        cp "${PACK_DIR}/Instance/${MODE_PACKLIST}" "$out"
    elif [ -n "$MODE_LEVEL_GLOB" ]; then
        {
            echo "$MODE_PLAYLIST"
            ( cd "${PACK_DIR}/dist/linux/levels" 2>/dev/null \
                && for d in $MODE_LEVEL_GLOB; do [ -d "$d" ] && echo "levels/${d}"; done )
        } > "$out"
    else
        warn "server ${n}: no map list found for this mode. Set SERVER_${n}_MAPLIST"
        echo "$MODE_PLAYLIST" > "$out"
    fi
    if [ "$(wc -l < "$out")" -lt 2 ]; then
        warn "server ${n}: the map list has no maps. Set SERVER_${n}_MAPLIST"
    fi
}

# Folder, ServerOptions.ini, Startup.txt and map list for server n.
# Returns 1 (and skips the server) if its TYPE is not valid.
gen_server() {
    local n=$1
    local tvar="SERVER_${n}_TYPE" type
    type=${!tvar}
    if ! mode_info "$type"; then
        warn "server ${n}: unknown TYPE '${type}' (use rush, conq, sqdm, sqrush, vietrush, vietconq, vietsqdm or vietsqrush). Skipping it."
        return 1
    fi

    local inst="${INSTANCE_ROOT}/${n}"
    mkdir -p "${inst}/AdminScripts"

    # Files the server expects. Copied from the pack only if missing, so bans
    # and reserved slots survive restarts.
    local f
    for f in banlist.txt reservedslotslist.txt textchatmoderationlist.txt; do
        if [ ! -e "${inst}/${f}" ]; then
            if [ -e "${PACK_DIR}/Instance/${f}" ]; then
                cp "${PACK_DIR}/Instance/${f}" "${inst}/${f}"
            else
                : > "${inst}/${f}"
            fi
        fi
    done
    if [ ! -d "${inst}/pb" ]; then
        if [ -d "${PACK_DIR}/Instance/pb" ]; then cp -r "${PACK_DIR}/Instance/pb" "${inst}/pb"; else mkdir -p "${inst}/pb"; fi
    fi

    # ---- ServerOptions.ini ----
    local port rcon_port
    port=$(own "$n" PORT $((19567 + n - 1)))
    rcon_port=$(own "$n" RCON_PORT $((48888 + n - 1)))

    opt_reset
    opt_set Name               "$(setting "$n" NAME "BFBC2 Server ${n}")"
    opt_set Port               "$port"
    # RCON only opens with the "ip:port" form. Localhost by default.
    opt_set RemoteAdminPort    "$(setting "$n" RCON_BIND 127.0.0.1):${rcon_port}"
    opt_set RemoteAdminPassword "$(setting "$n" RCON_PASSWORD "$RCON_DEFAULT_PW")"
    opt_set PunkBuster         "$(norm_bool "$(setting "$n" PUNKBUSTER false)")"
    opt_set Ranked             "$(norm_bool "$(setting "$n" RANKED true)")"
    opt_set NumGameClientSlots "$(setting "$n" MAX_PLAYERS 16)"
    opt_set RevisionLevel      8
    opt_set RevisionKey        7C0A303E-F4D2-985E-763D-E7C41B1E06A3
    opt_set GameModID          "$MODE_GAMEMOD"

    # Anything else: SERVER_OPT_<Key> for all servers, SERVER_<n>_OPT_<Key> for one.
    local pre v
    for pre in "SERVER_OPT_" "SERVER_${n}_OPT_"; do
        for v in $(compgen -A variable "$pre" | sort); do
            opt_set "${v#"$pre"}" "${!v}"
        done
    done

    {
        echo "[Options]"
        for f in "${OPT_ORDER[@]}"; do echo "${f}=${OPT_VAL[$f]}"; done
    } > "${inst}/ServerOptions.ini"

    # ---- AdminScripts/Startup.txt (runs when the server starts) ----
    var_reset
    var_set hardCore          "$(norm_bool "$(setting "$n" HARDCORE false)")"
    var_set friendlyFire      "$(norm_bool "$(setting "$n" FRIENDLY_FIRE false)")"
    var_set teamBalance       "$(norm_bool "$(setting "$n" TEAM_BALANCE true)")"
    var_set killCam           "$(norm_bool "$(setting "$n" KILLCAM true)")"
    var_set miniMap           "$(norm_bool "$(setting "$n" MINIMAP true)")"
    var_set crossHair         "$(norm_bool "$(setting "$n" CROSSHAIR true)")"
    var_set 3dSpotting        "$(norm_bool "$(setting "$n" SPOTTING_3D true)")"
    var_set miniMapSpotting   "$(norm_bool "$(setting "$n" MINIMAP_SPOTTING true)")"

    local desc pass banner
    desc=$(setting "$n" DESCRIPTION "")
    pass=$(setting "$n" GAME_PASSWORD "")
    banner=$(setting "$n" BANNER_URL "")
    [ -z "$desc" ]   || var_set serverDescription "${desc//$'\n'/|}"   # new lines become | in this game
    if [ -n "$pass" ]; then
        var_set gamePassword "$pass"
        # Found in testing: the game does not apply a password on a ranked server.
        if [ "${OPT_VAL[Ranked],,}" = true ]; then
            warn "server ${n}: GAME_PASSWORD is set, but this server is ranked and the game ignores passwords on ranked servers. Set SERVER_${n}_RANKED=false to use a password."
        fi
    fi
    [ -z "$banner" ] || var_set bannerUrl "$banner"

    # Anything else: SERVER_VAR_<name> for all servers, SERVER_<n>_VAR_<name> for one.
    for pre in "SERVER_VAR_" "SERVER_${n}_VAR_"; do
        for v in $(compgen -A variable "$pre" | sort); do
            var_set "${v#"$pre"}" "${!v}"
        done
    done

    {
        echo "# Generated at container start from environment variables. Do not edit."
        for f in "${VAR_ORDER[@]}"; do echo "vars.${f} ${VAR_VAL[$f]}"; done
        # Raw extra lines, if any (SERVER_<n>_STARTUP, else SERVER_STARTUP).
        local extra_all
        extra_all=$(setting "$n" STARTUP "")
        [ -z "$extra_all" ] || printf '%s\n' "$extra_all"
    } > "${inst}/AdminScripts/Startup.txt"

    gen_maplist "$n" "$inst"

    log "server ${n}: type=${type} name='${OPT_VAL[Name]}' game port=${port} RCON=${OPT_VAL[RemoteAdminPort]}"
}

# Everything, in the right order. Fills the SERVERS array with the valid servers.
generate_all() {
    mkdir -p "$DATA_DIR" "$INSTANCE_ROOT" "$CONFIG_DIR" "$MASTERDIR"
    gen_rcon_default
    gen_hook
    gen_master_config

    # The game looks for instance folders relative to its own folder. Keep the
    # real folders outside the pack (so a pack update never touches them) and
    # link them in.
    ln -sfn "$INSTANCE_ROOT" "${PACK_DIR}/instances"

    SERVERS=()
    local n
    for n in $(list_servers); do
        if gen_server "$n"; then SERVERS+=("$n"); fi
    done

    if [ "${#SERVERS[@]}" -eq 0 ]; then
        warn "No game servers defined. Set SERVER_1_TYPE (for example rush) to create one."
    fi
    check_ports
}

# Warn if two servers use the same port.
check_ports() {
    local -A seen=()
    local n key val
    for n in "${SERVERS[@]}"; do
        for key in PORT RCON_PORT; do
            case "$key" in
                PORT)      val=$(own "$n" PORT $((19567 + n - 1))) ;;
                RCON_PORT) val=$(own "$n" RCON_PORT $((48888 + n - 1))) ;;
            esac
            if [ -n "${seen[$val]+x}" ]; then
                warn "port ${val} is used twice (server ${seen[$val]} and server ${n} ${key}). Give each server its own ports."
            fi
            seen[$val]="${n} ${key}"
        done
    done
}
