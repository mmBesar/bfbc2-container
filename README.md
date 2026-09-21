<div align="center">

# BFBC2 All-in-One

**A complete Battlefield: Bad Company 2 LAN server in a single container,<br>configured entirely with environment variables.**

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)
[![Test](https://github.com/mmBesar/bfbc2-container/actions/workflows/test-aio.yml/badge.svg)](https://github.com/mmBesar/bfbc2-container/actions/workflows/test-aio.yml)
[![Publish](https://github.com/mmBesar/bfbc2-container/actions/workflows/publish.yml/badge.svg)](https://github.com/mmBesar/bfbc2-container/actions/workflows/publish.yml)
[![Image](https://img.shields.io/badge/ghcr.io-mmbesar%2Fbfbc2--container-2496ED?logo=docker&logoColor=white)](https://github.com/mmBesar/bfbc2-container/pkgs/container/bfbc2-container)
![Platform](https://img.shields.io/badge/platform-linux%2Famd64-lightgrey?logo=linux&logoColor=white)
![Config](https://img.shields.io/badge/config-100%25%20environment%20variables-brightgreen)
![Status](https://img.shields.io/badge/status-experimental-orange)
![Personal project](https://img.shields.io/badge/project-personal%20work-8A2BE2)

</div>

> [!NOTE]
> **This is a personal project.** I built it for my own LAN server and my own
> homelab, in my spare time, and I share it in case it is useful to others.
> It comes **as is, with no warranty and no promise of support**. It is **not**
> affiliated with or endorsed by EA, DICE, or the authors of the projects it
> builds on (all credited [below](#credits-and-sources)). Issues and ideas are
> welcome, but I can't promise quick answers.

## What is inside

- The **master server emulator** (MASE): accounts and the server list. It is
  compiled from its published source when the image is built, into one static
  program.
- **Any number of game servers**, run under Wine. All of them share one Wine
  setup and one fake screen, which uses much less memory than one container per
  server.
- A small start script that turns environment variables into every config
  file, restarts a server if it stops, and shuts everything down cleanly.
- A **web interface** (a small Go program) to see and manage the servers from a
  browser: players, kick and ban, round control, settings, and a console.

The game server files (about 440 MB) are **not** in the image. On first start
the container downloads them once, checks their SHA256, and unpacks them into
`/data/pack`. You can also use your own copy of the file (see
[Use your own copy of the server pack](#use-your-own-copy-of-the-server-pack)).

## Requirements

- Linux on **x86-64**. The game server is a 32-bit Windows program running
  under Wine, so ARM machines are not supported.
- Docker with Compose.
- `network_mode: host`. Port mapping breaks server registration.
- A legitimate copy of the game on every client, patched to version R11.

## Quick start

The repository's [`docker-compose.yml`](docker-compose.yml) is the whole
manual in one file: every option is there, with a short comment. By default it
starts **all eight server types** (four Bad Company 2 modes and four Vietnam
modes). Comment out the ones you do not want.

1. Copy `docker-compose.yml` to your machine.
2. Change the line marked `<-- CHANGE ME` (the RCON password: letters and
   digits only).
3. Start it:

   ```sh
   docker compose up -d
   docker compose logs -f
   ```

The first start takes a few minutes while the server pack downloads.

### Use your own copy of the server pack

You do not have to download the pack. If you already have `Bc2emu_V09.rar`:

1. Copy the **file** into the folder you mount at `/data`, so it appears as
   `./data/Bc2emu_V09.rar` on the host.
2. Add `PACK_FILE: "/data/Bc2emu_V09.rar"` to the environment.

Nothing else needs mounting, because `./data` is already mounted. On start the
container checks the file's SHA256 (it refuses any other file), unpacks it once
into `./data/pack`, and does not touch the `.rar` again. Keep it there as your
backup. If you prefer to keep it elsewhere, mount just that one file:
`- /path/to/Bc2emu_V09.rar:/data/Bc2emu_V09.rar:ro`.

### Want an image with the pack already inside?

The published image is small and does **not** contain the game server files.
That is on purpose: the pack holds EA's game server program and level data, and
I would rather not host those in a public image. It costs you one download,
once. After that the files live in your `./data` folder and are never fetched
again, not even when you update the image.

If you want an image that needs no download at all (for example for several
machines in your own network), build it yourself:

```sh
git clone https://github.com/mmBesar/bfbc2-container.git
cd bfbc2-container
docker build --target bundled -t bfbc2-bundled .
```

The build downloads the pack and checks its SHA256, and the container still
unpacks it into `./data/pack` on first start. Keep that image for your own use
or push it to a registry of your own. Please do not publish it publicly.

## Web interface

Open `http://<this machine's IP>:5010` in a browser and log in (user `admin`,
and the password you set with `WEB_PASSWORD`).

- **Overview:** one card per server with its map (a coloured tile, or a picture
  if you enable pictures), player count and state, and whether the master server
  is running.
- **Players:** the live player list with **Kick**, **Ban** (permanent, until the
  round ends, or for a number of seconds), and moving a player to the other
  team or to a squad.
- **Start, stop, restart:** switch a single game server on or off (to save CPU
  and memory) or restart it. This lasts until the container restarts; after
  that `SERVER_<n>_AUTOSTART` decides again.
- **Round & map:** next round, restart, end the round with a winner, and the
  server's map list.
- **Settings:** switch hardcore, friendly fire, killcam and the others on or
  off, and change the server name, description, password and more.
- **Console:** send any RCON command by hand.

Changes made in the web interface apply to the running server at once but are
**not kept after a restart**, because the settings files are rebuilt from
`docker-compose.yml` at every start. To make a change permanent, set it in
Compose.

The web interface talks to each server over RCON on `127.0.0.1`, so RCON never
has to be opened to the network. It uses plain HTTP with a password: **keep it
on your LAN or VPN**, or put it behind a reverse proxy with HTTPS.

## Connecting a client

The game needs two small files in its install folder, next to the game
executable. After the first start you will find them in
`./data/pack/Crack - Copy to client root/`:

- `dinput8.dll`
- `bfbc2.ini`

Open `bfbc2.ini` and set `host=` to the IP address of the machine running this
container. Start the game, create an account with any name and password (it is
a local account, kept in plain text in `./data/master/database`), and pick a
server from the list.

## Environment variables

### General

| Variable | Default | Meaning |
|---|---|---|
| `PUID`, `PGID` | `1000` | User and group ID that own everything in `/data` |
| `TZ` | `UTC` | Time zone, for example `Europe/Berlin` |
| `PACK_FILE` | not set | Path (inside the container) to your own copy of `Bc2emu_V09.rar`. Nothing is downloaded. |
| `PACK_URL` | SourceForge | Where to download the pack from |
| `PACK_SHA256` | built in | The pack must match this SHA256 |
| `WINEDEBUG` | `fixme-all,err-vulkan` | Wine's debug output |

### Master server

| Variable | Default | Meaning |
|---|---|---|
| `MASTER_ENABLED` | `true` | Set `false` to run only game servers (they then look for a master elsewhere) |
| `MASTER_HOST` | `127.0.0.1` | Where the game servers find the master |
| `MASTER_<KEY>` | | Any setting of the master's `config.ini`, in upper case. Example: `MASTER_ALL_STATS_UNLOCKED=true` sets `all_stats_unlocked = true`. Unknown keys are reported in the log. |
| `MASTER_EMULATOR_IP` | this machine's LAN address, found automatically | Address the master tells clients to connect to. Set it yourself only if the detected address is wrong. **Never use `0.0.0.0`**: clients can then not log in. |
| `MASTER_LOG_CREATE` | `true` | Write the master's log file (`./data/master/logfile.log`) |
| `MASTER_FILE_LOG_LEVEL` | `1` | Log detail from 1 (connections) to 3 (everything). Use 3 when debugging. |
| `MASTER_LOG_TO_CONSOLE` | `true` | Also show the master's log file in `docker logs` |

Useful master keys: `LOG_CREATE`, `CONSOLE_LOG_LEVEL`, `ALL_STATS_UNLOCKED`,
`ALL_ARE_VETERAN`, `PREMIUM_FOR_ALL`, `SPECACT_FOR_ALL`, `VIETNAM_FOR_ALL`,
`ENABLE_SERVER_FILTERS`, `HTTP_ENABLED`, and the port settings.

### Web interface

| Variable | Default | Meaning |
|---|---|---|
| `WEB_ENABLED` | `true` | Set `false` to turn the web interface off |
| `WEB_PORT` | `5010` | Port of the web interface |
| `WEB_USER` | `admin` | Login name |
| `WEB_PASSWORD` | random | Login password. If not set, a random one is made once and saved in `/data/config/web-password`. |
| `WEB_BIND` | `0.0.0.0` | Address the web interface listens on |
| `WEB_MAP_IMAGES` | `false` | Also show map **pictures**. When `true`, each is downloaded once from the PRoCon project into `/data/cache/maps` (not part of the image). Without it, maps get a coloured tile with their name and the container needs no outside source. |

### Game servers

A server exists as soon as `SERVER_<n>_TYPE` is set (`n` = 1, 2, 3, ...).

For each setting below, `SERVER_<n>_<KEY>` applies to one server and
`SERVER_<KEY>` sets a default for all servers. The per-server value wins.
The keys marked "per server only" have no global form.

| Key | Default | Meaning |
|---|---|---|
| `TYPE` (per server only) | | `rush`, `conq`, `sqdm`, `sqrush`, `vietrush`, `vietconq`, `vietsqdm`, `vietsqrush` |
| `NAME` | `BFBC2 Server <n>` | Name in the server list |
| `PORT` (per server only) | `19567 + (n-1)` | Game port |
| `RCON_PORT` (per server only) | `48888 + (n-1)` | Remote admin port |
| `RCON_BIND` | `127.0.0.1` | Address RCON listens on. Use `0.0.0.0` to allow other machines. |
| `RCON_PASSWORD` | random | Remote admin password: **letters and digits only** (the game refuses anything else and stops). If no server has one, a random one is made once and saved in `/data/config/rcon-password`. |
| `MAX_PLAYERS` | `16` | Player slots |
| `PUNKBUSTER` | `false` | |
| `RANKED` | `true` | |
| `REGION` | `OC` | Region code passed to the server |
| `HEARTBEAT` | `20000` | Heartbeat interval in milliseconds |
| `MAPPACK2` | `1` | Enable map pack 2 |
| `MAPLIST` (per server only) | the mode's default | Comma-separated maps, for example `mp_001, mp_003`. Names without a folder get `levels/` added. |
| `HARDCORE`, `FRIENDLY_FIRE` | `false` | |
| `TEAM_BALANCE`, `KILLCAM`, `MINIMAP`, `CROSSHAIR`, `SPOTTING_3D`, `MINIMAP_SPOTTING` | `true` | |
| `DESCRIPTION` | none | Server description (line breaks are allowed) |
| `GAME_PASSWORD` | none | Password to join. Only works on unranked servers (`RANKED=false`); the game ignores it on ranked ones. |
| `BANNER_URL` | none | Server banner image address |
| `AUTOSTART` | `true` | `false` = the server is defined but stays stopped until you start it in the web interface |
| `STARTUP` | none | Extra raw lines for the server's startup script |
| `EXTRA_ARGS` | none | Extra command line arguments for the server program |

Anything not listed can still be set:

| Variable | Meaning |
|---|---|
| `SERVER_<n>_OPT_<Key>` (or `SERVER_OPT_<Key>`) | Any key of that server's `ServerOptions.ini`, for example `SERVER_2_OPT_Ranked=false` |
| `SERVER_<n>_VAR_<name>` (or `SERVER_VAR_<name>`) | Any `vars.<name>` startup setting, for example `SERVER_2_VAR_idleTimeout=300` |

Boolean values accept `true/false`, `1/0`, `yes/no`, `on/off`.

### Gameplay options shared by all servers

These live in one file next to the game program, so they apply to every
server in the container.

`HOOK_RANKING_MIN_PLAYERS` (1), `HOOK_DESERTING_ALLOWED` (0),
`HOOK_INSTANT_SPAWN` (0), `HOOK_UNLIMITED_AMMO` (0), `HOOK_HEALTH_MODE` (0).

## Using less CPU and memory

On a small machine the number of running game servers is what matters. From
measurements of this container:

| Running servers | Memory | CPU when nobody is playing |
|---|---|---|
| 3 | about 0.5 GB | roughly 15-20% of one core |
| 8 | about 1 GB | roughly 45-60% of one core |

There is a fixed part of about 200 MB (Wine and the fake screen) and each
server adds roughly 100-150 MB and 5-7% of a core, even when empty, because it
keeps its map loaded and keeps ticking. The master server and the web
interface need very little.

What helps, from most to least effective:

1. **Run fewer servers.** Comment out the ones you never play.
2. **Keep rarely used servers stopped.** Set `SERVER_<n>_AUTOSTART: "false"`
   (or `SERVER_AUTOSTART: "false"` for all) and start a server from the web
   interface only when you want it. A stopped server uses no CPU or memory
   and is not in the game's server list.
3. **Give the container limits** so it can never crowd out other services:
   `cpus`, `cpu_shares` and `mem_limit` (see the commented lines in
   `docker-compose.yml`). They cap or de-prioritise the container. They do not
   make it use less by themselves.

## Map pictures

The web interface never needs pictures: every map gets a coloured tile with its
name. If you want real pictures, there are two ways:

- Set `WEB_MAP_IMAGES: "true"`. Each picture is downloaded once from the PRoCon
  project (a fixed version of it) and kept in `./data/cache/maps`.
- Or put your own JPEG files in `./data/cache/maps`, named after the level, for
  example `mp_002.jpg`. Your files always win.

The pictures are artwork of the game and are **not** included in this
repository or the image.

## The data folder

Mount one folder at `/data`:

| Path | Contents |
|---|---|
| `/data/pack` | The unpacked server pack (safe to delete; it is unpacked again) |
| `/data/instances/<n>` | One folder per game server: generated settings, ban list, reserved slots |
| `/data/master` | The master's generated config, its templates, and the account database |
| `/data/config` | The generated RCON and web passwords, if you did not set them |
| `/data/cache/maps` | Map pictures (only if you enable them or add your own) |

Settings files are **regenerated from your environment variables on every
start**, so change them in Compose, not in the files. Ban lists, reserved
slots and accounts are kept. Back up this folder.

## Ports and firewall

The master listens on TCP 18390, 18395, 19021 and 19026 by default (and
optionally 9946). Each game server needs its own game port. RCON is bound to
`127.0.0.1` unless you change `RCON_BIND`. The web interface uses TCP 5010
by default (`WEB_PORT`).
## Troubleshooting

| Problem | Likely cause and fix |
|---|---|
| I can create an account but not log in | The master tells the client where to connect next using `MASTER_EMULATOR_IP`. It must be this machine's real LAN address (never `0.0.0.0`, never a made-up one). Check `emulator_ip` in `./data/master/config.ini`. |
| I can log in but the server list is empty | The game servers did not register. Look at the end of `./data/instances/1/RuntimeLog_*.log` (newest file) for a line with `FatalAssert`. A common cause is an RCON password with a dash, space or symbol: use letters and digits only. |
| Nothing in `docker logs` from the master | The master's log file is shown there at level 1. Set `MASTER_FILE_LOG_LEVEL: "3"` for full detail, or read `./data/master/logfile.log`. |
| I want to see what a server is doing | `./data/instances/<n>/RuntimeLog_*.log` is that server's own log. |

## Security notes

- This is meant for a **LAN or a VPN**. Do not expose the master to the
  internet: it speaks very old encryption and stores accounts in plain text.
- Set your own RCON password, and leave `RCON_BIND` at `127.0.0.1` unless you
  need remote administration.

## Known limitations

- x86-64 only (Wine, 32-bit game server).
- Host networking only.
- The web interface cannot yet edit the ban list or the map list, or manage
  master accounts. These are planned.

## Tests and releases

Automated tests run in GitHub Actions and are started by hand from the Actions
tab.

- **`test-aio`** builds the image, starts three servers from environment
  variables alone, and checks over RCON that the settings arrived, that
  `PUID`/`PGID`/`TZ` work, that all servers register with the master, and that a
  restart is clean and does not download the pack again. It also checks the
  web interface (login, server list, and a command sent through it).
- **`publish`** runs that whole test first, scans the image with Trivy (fails on
  any fixable CRITICAL vulnerability), and only then pushes the image to the
  GitHub Container Registry with an SBOM and build provenance. It finally checks
  that the image can be pulled by anyone without logging in.

## Credits and sources

This project is only glue. All the hard work was done by others. Thank you!

| Who / what | What it is and how it is used here |
|---|---|
| **Triver** and the **BFBC2 MASE** project ([SourceForge](https://sourceforge.net/projects/battlefieldbadcompany2mase/), project page by flyer8472) | The master server emulator (built from its published source) and the server pack with the client hook. This project would not exist without it. Its readme also thanks **Domo**, **Freaky123** and **Aluigi**. |
| **jkuettner**: [bfbc2-server](https://codeberg.org/jkuettner/bfbc2-server) (Codeberg; the [GitHub copy](https://github.com/jkuettner/bfbc2-server) is archived) | Docker images that showed how to run the master and the game servers in containers, and which Wine pieces are needed. |
| **The-May**: [bfbc2-webcon](https://github.com/The-May/bfbc2-webcon) | A web dashboard that showed how the remote admin (RCON) protocol behaves. Its map index was the reference for the map names and which picture belongs to which map. |
| **AdKats / PRoCon**: [Procon-1](https://github.com/AdKats/Procon-1) and **[OpenRCON](https://github.com/OpenRcon/OpenRcon)** | Remote admin tools known to work with this stack, and references for the RCON protocol. If you enable `WEB_MAP_IMAGES`, the web interface downloads map pictures from the PRoCon repository (once, at run time; they are not stored in this repository or the image). The pictures are artwork of the game. |
| **[Wine](https://www.winehq.org/)**, **[winetricks](https://github.com/Winetricks/winetricks)**, **[Debian](https://www.debian.org/)**, **[Xvfb](https://www.x.org/)** | Run the Windows game server on Linux. |
| **[tini](https://github.com/krallin/tini)**, **[gosu](https://github.com/tianon/gosu)** | Correct process handling and the PUID/PGID user switch in the container. |
| **[Trivy](https://github.com/aquasecurity/trivy)**, the **Docker GitHub Actions** | Vulnerability scanning and image builds in CI. |
| **[Go](https://go.dev/)** | The web interface is written in Go using only its standard library. |

Related projects I do not use, but which are worth knowing:
[GrzybDev/BFBC2_MasterServer](https://github.com/GrzybDev/BFBC2_MasterServer)
(a modern master server written in Python) and **Project Rome** (a
community-run online backend for the game).

Battlefield: Bad Company 2 is a product of EA / DICE. All trademarks belong to
their owners. This project contains **no game files**: the server pack is
downloaded from its original public location, and you must own a legitimate
copy of the game to play.

## License

The files in this repository (the Dockerfile, scripts, workflows and
documentation) are licensed under the
**[GNU Affero General Public License v3.0 or later](LICENSE)** (AGPL-3.0-or-later).
You are free to use, study, change and share them, and if you share a changed
version, or run one as a service for others, you must keep it open under the
same license.

The third-party software this image uses keeps its own licenses. In particular:

- **MASE** has no separate license file. Its readme invites others to change
  the code and release the result, and asks only for credit. It is compiled
  from that published source at build time and credited above. The only change
  made to its code is a one-line fix for the log timestamps (visible in the
  `Dockerfile`). If you are one of its authors and would like anything done
  differently, please open an issue.
- The **server pack** is not part of this repository or the image.
