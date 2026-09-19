# =============================================================================
# BFBC2 master server emulator (MASE) - BUILD TEST
#
# In plain words:
#   1. Download the emulator's source code from the SourceForge project and
#      check it is exactly the file we expect (SHA256).
#   2. Compile it.
#   3. Copy only the finished program into a small final image.
#
# This is only a build test. PUID/PGID/TZ handling, config, and the database
# volume come later, in the real all-in-one image.
#
# Credit: MASE is by Triver (SourceForge: battlefieldbadcompany2mase).
# =============================================================================


# ---- Stage 1: download the source and prepare a Makefile ---------------------
FROM debian:bookworm-slim AS source

ARG SRC_URL="https://downloads.sourceforge.net/project/battlefieldbadcompany2mase/Source%20V0.9/src_v09.rar"
# Passed in by the workflow. The build FAILS if the download does not match.
ARG SRC_SHA256

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
RUN sed -i -e 's@$(BOOST_1_54_DIR)@'${LIB_BOOST_DIR}'@g' mase_bc2.vcxproj mase_bc2.cbp \
 && sed -i -e 's@$(OPEN_SSL_DIR)@'${LIB_OPENSSL_DIR}'@g' mase_bc2.vcxproj mase_bc2.cbp \
 && for lib in libboost_system libboost_filesystem libboost_random libssl libcrypto; do \
      sed -i -e "s@${lib}.a@${LIB_PATH}/${lib}.a@g" mase_bc2.cbp; \
    done \
 && cbp2make -in mase_bc2.cbp -out Makefile \
 && sed -i -e '/^LIB = / s@$@ -lz@' Makefile


# ---- Stage 2: compile --------------------------------------------------------
# Old Alpine on purpose: it is the toolchain the existing community Docker
# build uses successfully, so we keep it until we prove a newer one works.
# It is only used to build. The finished program is copied out, so this old
# system does NOT end up in the final image.
FROM alpine:3.8 AS build

ENV CXXFLAGS="-std=gnu++11"

RUN apk add --no-cache bash make gcc g++ boost-dev openssl-dev

COPY --from=source /bfbc2 /bfbc2
WORKDIR /bfbc2/src
RUN make release


# ---- Stage 3: small final image ----------------------------------------------
FROM alpine:3.22

# Runtime libraries the compiled program needs
RUN apk add --no-cache libstdc++ libgcc zlib

COPY --from=build /bfbc2/src/bin/Release/mase_bc2 /opt/mase/mase_bc2

WORKDIR /opt/mase
CMD ["/opt/mase/mase_bc2"]
