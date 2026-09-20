# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 mmBesar
# =============================================================================
# BFBC2 all-in-one image
#
# ONE container with everything for a Battlefield: Bad Company 2 LAN server:
#   - the master server emulator (MASE), compiled from source
#   - any number of game servers, run under Wine
#   - one shared fake screen for them
# Everything is controlled by environment variables (see docker-compose.example.yml).
#
# In plain words, the build has three stages:
#   1. "source": download the master server's source code and verify it.
#   2. "build":  compile it into one static program.
#   3. final:    a Debian image with Wine, plus that program and our scripts.
# The game server files are NOT baked in. On first start the container
# downloads them (or uses your own copy) into /data/pack and verifies them.
# (An optional "bundled" build target that does include them exists, but it
# is never published. See the end of this file.)
#
# Credits:
#   - MASE (master server emulator and server pack): Triver, on SourceForge
#     (battlefieldbadcompany2mase). Server pack thanks also to Domo, Freaky123.
#   - jkuettner's bfbc2-server and The-May's bfbc2-webcon showed how to run
#     this stack in containers and how the admin protocol behaves.
# =============================================================================


# ---- Stage 1: download the master server source and prepare a Makefile -------
FROM debian:bookworm-slim AS source

ARG SRC_URL="https://downloads.sourceforge.net/project/battlefieldbadcompany2mase/Source%20V0.9/src_v09.rar"
# SHA256 of src_v09.rar. The build FAILS if the download does not match.
ARG SRC_SHA256="a3e180526ac71af639e73c5ee33ea98c55ddae48d0e81aea2621070b0272031c"

# Where Boost and OpenSSL live inside the build image (stage 2)
ENV LIB_BOOST_DIR="/usr/include/boost/" \
    LIB_OPENSSL_DIR="/usr/include/openssl/" \
    LIB_PATH="/usr/lib"

# unrar is in Debian's "non-free" section, so enable that first.
# cbp2make turns the project's Code::Blocks file (.cbp) into a normal Makefile.
RUN sed -i 's/^Components: main$/& contrib non-free non-free-firmware/' /etc/apt/sources.list.d/debian.sources \
 && apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl unrar cbp2make \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /bfbc2

# Download, verify, unpack
RUN curl -fL --retry 3 -o /tmp/src.rar "${SRC_URL}" \
 && echo "${SRC_SHA256}  /tmp/src.rar" | sha256sum -c - \
 && unrar x /tmp/src.rar \
 && rm /tmp/src.rar

WORKDIR /bfbc2/src

# The project files point at the original author's own library folders.
# Point them at the ones in our build image (stage 2) instead.
# Also link the program fully STATIC (-static), so it carries everything it
# needs and runs in the Debian-based final image.
RUN sed -i -e 's@$(BOOST_1_54_DIR)@'${LIB_BOOST_DIR}'@g' mase_bc2.vcxproj mase_bc2.cbp \
 && sed -i -e 's@$(OPEN_SSL_DIR)@'${LIB_OPENSSL_DIR}'@g' mase_bc2.vcxproj mase_bc2.cbp \
 && for lib in libboost_system libboost_filesystem libboost_random libssl libcrypto; do \
      sed -i -e "s@${lib}.a@${LIB_PATH}/${lib}.a@g" mase_bc2.cbp; \
    done \
 && cbp2make -in mase_bc2.cbp -out Makefile \
 && sed -i -e '/^LIB = / s@$@ -lz@' Makefile \
 && sed -i -e '/^LDFLAGS = / s@$@ -static@' Makefile


# ---- Stage 2: compile --------------------------------------------------------
# Old Alpine on purpose: it is the toolchain the existing community Docker
# build uses successfully, so we keep it until we prove a newer one works.
# It is only used to build. The finished program is copied out, so this old
# system does NOT end up in the final image.
FROM alpine:3.8 AS build

ENV CXXFLAGS="-std=gnu++11"

RUN apk add --no-cache bash make gcc g++ boost-dev openssl-dev zlib-dev

COPY --from=source /bfbc2 /bfbc2
WORKDIR /bfbc2/src
RUN make release

# Safety check: the finished program must be fully static (needs no shared
# libraries). If it is not, stop the build here with a clear message.
RUN if readelf -d bin/Release/mase_bc2 | grep -q NEEDED; then \
      echo "ERROR: mase_bc2 is not statically linked. It still needs:"; \
      readelf -d bin/Release/mase_bc2 | grep NEEDED; \
      exit 1; \
    fi


# ---- Stage 3: the runtime image ----------------------------------------------
FROM debian:bookworm-slim AS runtime

ENV DEBIAN_FRONTEND=noninteractive

# Wine needs the 32-bit (i386) architecture enabled because the game server is
# a 32-bit program. "non-free" is needed for the unrar package.
#   wine32          - Wine for 32-bit Windows programs
#   xvfb, xauth     - the fake screen
#   winetricks      - installs Windows runtime libraries into Wine
#   cabextract, winbind, libgl1-mesa-glx - helpers Wine/winetricks expect
#   unrar, wget     - unpack and download the server pack
#   tzdata          - time zones (TZ variable)
#   gosu            - run the servers as your PUID/PGID user
#   tini            - proper PID 1: forwards stop signals, cleans up processes
#   procps          - pkill, used when shutting down
#   iproute2        - finds this machine's LAN address (MASTER_EMULATOR_IP)
RUN dpkg --add-architecture i386 \
 && sed -i 's/^Components: main$/& contrib non-free non-free-firmware/' /etc/apt/sources.list.d/debian.sources \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
        wine32 \
        xvfb \
        xauth \
        winetricks \
        cabextract \
        winbind \
        libgl1-mesa-glx:i386 \
        ca-certificates \
        unrar \
        wget \
        tzdata \
        gosu \
        tini \
        procps \
        iproute2 \
 && rm -rf /var/lib/apt/lists/*

# A normal (non-root) user to run everything as, plus the /data folder that
# will hold the server pack, all server settings and all state.
# (The entrypoint changes this user's ID to your PUID/PGID at start.)
RUN useradd -m -u 1000 bfbc2 \
 && mkdir /data \
 && chown bfbc2:bfbc2 /data

# Build the Wine environment once, now, so containers start fast:
#   - wineboot creates it
#   - winetricks adds the Windows libraries the game server needs:
#     dinput8 (lets the emulator's hook DLL load) and the Visual C++ runtimes
#     2005, 2008 and 2010.
# WINEDEBUG=-all keeps the build log quiet.
# "wineserver -w" waits until Wine has finished writing its settings.
# The last step deletes winetricks' download cache (the installers it used).
# They are not needed any more and would only make the image bigger.
USER bfbc2
ENV WINEARCH=win32 \
    WINEPREFIX=/home/bfbc2/.wine32
RUN WINEDEBUG=-all xvfb-run -e /dev/stdout -a -s "-nolisten tcp -screen 0 1280x1024x24" wineboot \
 && WINEDEBUG=-all xvfb-run -e /dev/stdout -a -s "-nolisten tcp -screen 0 1280x1024x24" \
        winetricks -q dinput8 vcrun2005 vcrun2008 vcrun2010 \
 && wineserver -w \
 && rm -rf /home/bfbc2/.cache/winetricks

# The entrypoint starts as root (to apply PUID/PGID/TZ), then drops to "bfbc2".
USER root

# The Wine setup above leaves files in /tmp that belong to user 1000. If you
# run with another PUID, Wine would find them and fail with "Permission
# denied". So empty /tmp now. (The entrypoint also cleans it at every start.)
RUN rm -rf /tmp/* /tmp/.[!.]* || true

# The compiled master server and our scripts. These come AFTER the slow Wine
# setup on purpose: changing a script then only rebuilds these last layers
# instead of the whole Wine environment.
# Note: COPY --chmod also applies to any folder it creates. The folder for
# lib.sh must be readable by everyone, so lib.sh uses 755 too (it is only
# read, not run, but 755 keeps its folder open).
COPY --from=build /bfbc2/src/bin/Release/mase_bc2 /opt/mase/mase_bc2
COPY --chmod=755 scripts/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY --chmod=755 scripts/start.sh      /usr/local/bin/start.sh
COPY --chmod=755 scripts/fetch-pack.sh /usr/local/bin/fetch-pack
COPY --chmod=755 scripts/lib.sh        /usr/local/lib/bfbc2/lib.sh

# Defaults. Every one of these can be changed with environment variables.
#   PACK_SHA256: SHA256 of the server pack (Bc2emu_V09.rar) that must be used.
#   WINEDEBUG:   hide Wine's harmless "fixme" notes and its Vulkan warning.
ENV PUID=1000 \
    PGID=1000 \
    TZ=UTC \
    PACK_SHA256=d1b25860f23af15cbab6549d41840836056ab38a49357eb3064a7d00f6d9f04b \
    WINEDEBUG=fixme-all,err-vulkan

# Descriptive labels. (The link to the source repository is added by the
# publish workflow, because it depends on where the repo lives.)
LABEL org.opencontainers.image.title="BFBC2 All-in-One" \
      org.opencontainers.image.description="Battlefield: Bad Company 2 LAN server (master + game servers under Wine) in one container, configured only by environment variables" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later"

WORKDIR /data

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]


# =============================================================================
# Two ways to finish the image. The LAST stage of this file is the default.
#
#   slim     (default)  The small image that gets published. It does NOT contain
#                       the game server files. On first start it downloads them
#                       once into /data (or uses your own copy: PACK_FILE).
#
#   bundled  (optional) An image with the server pack (Bc2emu_V09.rar) already
#                       inside, so it needs no download at all. It is NOT
#                       published, because the pack contains EA's game server
#                       program and level data, which we do not redistribute.
#                       Build it yourself for your own machines:
#                           docker build --target bundled -t bfbc2-bundled .
# =============================================================================

# ---- The pack, downloaded and verified (only used by "bundled") --------------
FROM debian:bookworm-slim AS pack

ARG PACK_URL="https://downloads.sourceforge.net/project/battlefieldbadcompany2mase/Bc2emu_V09.rar"
# SHA256 of Bc2emu_V09.rar. The build FAILS if the download does not match.
ARG PACK_SHA256="d1b25860f23af15cbab6549d41840836056ab38a49357eb3064a7d00f6d9f04b"

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates wget \
 && rm -rf /var/lib/apt/lists/* \
 && mkdir /pack \
 && wget -nv --tries=3 -O /pack/Bc2emu_V09.rar "${PACK_URL}" \
 && echo "${PACK_SHA256}  /pack/Bc2emu_V09.rar" | sha256sum -c -

# ---- bundled: runtime + the pack file -----------------------------------------
# The container still unpacks it once into /data/pack on first start, using the
# same code path as PACK_FILE. Only the download is gone.
FROM runtime AS bundled
COPY --from=pack /pack/Bc2emu_V09.rar /opt/pack/Bc2emu_V09.rar
ENV PACK_FILE=/opt/pack/Bc2emu_V09.rar

# ---- slim: the default, small image -----------------------------------------
FROM runtime AS slim
