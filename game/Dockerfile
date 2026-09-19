# =============================================================================
# BFBC2 game server "engine" image - BUILD TEST
#
# In plain words:
#   The BFBC2 dedicated server is a Windows program. To run it on Linux we use
#   Wine (a Windows-compatibility layer) plus Xvfb (a fake screen, because the
#   program expects a display to exist).
#
#   This image contains ONLY the tools: Wine, Xvfb, the Windows runtime
#   libraries the game needs, and a helper script ("fetch-pack") that
#   downloads and unpacks the game server files at run time into /data/pack.
#   The game files themselves are NOT baked into this image.
#
# This is only a build test. PUID/PGID/TZ handling and the supervisor come
# later, in the real all-in-one image.
#
# Credit: the server pack comes from the MASE project by Triver (SourceForge:
# battlefieldbadcompany2mase). The Wine setup follows the approach shown in
# jkuettner's bfbc2-server Docker images.
# =============================================================================

FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive

# Wine needs the 32-bit (i386) architecture enabled because the game server is
# a 32-bit program. "non-free" is needed for the unrar package.
#   wine32          - Wine for 32-bit Windows programs
#   xvfb, xauth     - the fake screen
#   winetricks      - installs Windows runtime libraries into Wine
#   cabextract, winbind, libgl1-mesa-glx - helpers Wine/winetricks expect
#   unrar           - unpacks the server pack
#   wget            - downloads the server pack
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
 && rm -rf /var/lib/apt/lists/*

# A normal (non-root) user to run everything as, plus the /data folder
# that will hold the server pack and, later, all server settings and state.
RUN useradd -m -u 1000 bfbc2 \
 && mkdir /data \
 && chown bfbc2:bfbc2 /data

# Helper script that downloads, verifies (SHA256) and unpacks the server pack.
COPY --chmod=755 scripts/fetch-pack.sh /usr/local/bin/fetch-pack

USER bfbc2

# 32-bit Wine environment, kept in the user's home folder
ENV WINEARCH=win32 \
    WINEPREFIX=/home/bfbc2/.wine32

# Build the Wine environment once, now, so containers start fast:
#   - wineboot creates it
#   - winetricks adds the Windows libraries the game server needs:
#     dinput8 (lets the emulator's hook DLL load) and the Visual C++ runtimes
#     2005, 2008 and 2010.
# WINEDEBUG=-all keeps the build log quiet.
# "wineserver -w" waits until Wine has finished writing its settings.
RUN WINEDEBUG=-all xvfb-run -e /dev/stdout -a -s "-nolisten tcp -screen 0 1280x1024x24" wineboot \
 && WINEDEBUG=-all xvfb-run -e /dev/stdout -a -s "-nolisten tcp -screen 0 1280x1024x24" \
        winetricks -q dinput8 vcrun2005 vcrun2008 vcrun2010 \
 && wineserver -w

WORKDIR /data

CMD ["bash"]
