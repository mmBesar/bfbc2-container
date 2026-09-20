# BFBC2 All-in-One

One container that runs a complete **Battlefield: Bad Company 2** LAN server:
the master server (accounts and server list), any number of game servers, all
configured **only through environment variables** in your Compose file.

> **Status: experimental.** The container is tested in CI (see below), but it
> has not yet been tested with a real game client. A web interface for managing
> the servers is planned.

## What is inside

- The **master server emulator** (MASE), compiled from source at build time
  into one static program.
- **Game servers**, run under Wine. All of them share one Wine setup and one
  fake screen, which saves a lot of memory compared with one container per
  server.
- A small start script that turns environment variables into all config files,
  restarts a server if it stops, and shuts everything down cleanly.

The game server files (about 440 MB) are **not** in the image. On first start
the container downloads them once from the MASE project on SourceForge,
checks their SHA256, and unpacks them into `/data/pack`. You can use your own
copy instead (`PACK_FILE`).

## Requirements

- Linux on **x86-64**. The game server is a 32-bit Windows program running
  under Wine, so ARM machines are not supported.
- Docker with Compose.
- `network_mode: host`. Port mapping breaks server registration.
- A legitimate copy of the game on every client, patched to version R11.

## Quick start

1. Copy `docker-compose.example.yml` to `docker-compose.yml` and adjust it.
2. Start it:

   ```sh
   docker compose up -d
   docker compose logs -f
   ```

3. The first start takes a few minutes while the server pack downloads.

## Connecting a client

The game needs two small files in its install folder (next to the game
executable). After the first start you will find them in
`./data/pack/Crack - Copy to client root/`:

- `dinput8.dll`
- `bfbc2.ini`

Open `bfbc2.ini` and set `host=` to the IP address of the machine running this
container. Start the game, create an account with any name and password (it
is a local account, kept in plain text in `./data/master/database`), and pick
a server from the list.

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
| `MASTER_EMULATOR_IP` | `0.0.0.0` | Address the master hands out to clients. Set it to your LAN IP if clients cannot see servers. |

Useful master keys: `LOG_CREATE`, `CONSOLE_LOG_LEVEL`, `ALL_STATS_UNLOCKED`,
`ALL_ARE_VETERAN`, `PREMIUM_FOR_ALL`, `SPECACT_FOR_ALL`, `VIETNAM_FOR_ALL`,
`ENABLE_SERVER_FILTERS`, `HTTP_ENABLED`, and the port settings.

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
| `RCON_PASSWORD` | random | Remote admin password. If no server has one, a random one is made once and saved in `/data/config/rcon-password`. |
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
| `GAME_PASSWORD` | none | Password to join |
| `BANNER_URL` | none | Server banner image address |
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

## The data folder

Mount one folder at `/data`:

| Path | Contents |
|---|---|
| `/data/pack` | The unpacked server pack (safe to delete; it is downloaded again) |
| `/data/instances/<n>` | One folder per game server: generated settings, ban list, reserved slots |
| `/data/master` | The master's generated config, its templates, and the account database |
| `/data/config` | The generated RCON password, if you did not set one |

Settings files are **regenerated from your environment variables on every
start**, so change them in Compose, not in the files. Ban lists, reserved
slots and accounts are kept.

## Ports and firewall

The master listens on TCP 18390, 18395, 19021 and 19026 by default (and
optionally 9946). Each game server needs its own game port. RCON is bound to
`127.0.0.1` unless you change `RCON_BIND`.

## Security notes

- This is meant for a **LAN or a VPN**. Do not expose the master to the
  internet: it speaks very old encryption and stores accounts in plain text.
- Set your own RCON password, and leave `RCON_BIND` at `127.0.0.1` unless you
  need remote administration.

## Known limitations

- x86-64 only (Wine, 32-bit game server).
- Host networking only.
- The master's log timestamps show wrong hours and minutes (cosmetic, from the
  original program).
- Not yet tested with a real game client.

## Tests

Automated tests run in GitHub Actions and are started by hand from the
Actions tab. `test-aio` builds the image, starts three servers from
environment variables alone, and checks over RCON that the settings arrived,
that `PUID`/`PGID`/`TZ` work, that all servers register with the master, and
that a restart is clean and does not download the pack again.

## Acknowledgements

Huge thanks to the people whose work this project builds on:

- **Triver**, developer of the **BFBC2 MASE** master server emulator and server
  pack ([SourceForge: battlefieldbadcompany2mase](https://sourceforge.net/projects/battlefieldbadcompany2mase/),
  project page by flyer8472). Its readme also thanks **Domo**, **Freaky123**
  and **Aluigi**.
- **jkuettner** - [bfbc2-server](https://codeberg.org/jkuettner/bfbc2-server),
  Docker images that showed how to run this stack in containers.
- **The-May** - [bfbc2-webcon](https://github.com/The-May/bfbc2-webcon),
  a web dashboard that showed how the remote admin protocol behaves.

Battlefield: Bad Company 2 is a product of EA / DICE. This project is not
affiliated with or endorsed by them.

## License

Not yet decided for this repository's own files.
