# BINNY active-extraction sandbox (STATIC tier).
#
# A disposable, offline container BINNY runs extraction scripts in. It only ever
# READS the sample (mounted read-only at /sample) as data — the sample is never
# executed here. The container is launched with --network none, non-root,
# --read-only, dropped caps, no-new-privileges, and cpu/mem/pids limits (see
# internal/sandbox/sandbox.go). The workspace at /work is a persistent host
# directory, so carved firmware trees and extracted configs survive sessions.
#
# Build (tag is the cache key — bump openbin/binny-extract:N in lockstep with
# sandbox.Image when this changes):
#   docker build -t openbin/binny-extract:3 binny/sandbox
#
#   :2 — binutils + python3 + radare2 + pycryptodome
#   :3 — adds binwalk + firmware-carving helpers (squashfs-tools, 7z, xz,
#        zstd, lzop, cpio, unzip) for arch identification / unpacking of raw
#        firmware images
FROM debian:bookworm-slim

# radare2 was dropped from Debian bookworm's repos (and isn't in backports), so
# install it from the project's pinned official .deb. curl/ca-certificates are
# build-time only — the runtime container is launched with --network none.
ARG R2_VERSION=5.9.8
RUN apt-get update && apt-get install -y --no-install-recommends \
      binutils \
      xxd \
      python3 \
      python3-pip \
      file \
      binwalk \
      squashfs-tools \
      p7zip-full \
      unzip \
      xz-utils \
      zstd \
      lzop \
      cpio \
      curl \
      ca-certificates \
    && curl -fsSL -o /tmp/radare2.deb \
      "https://github.com/radareorg/radare2/releases/download/${R2_VERSION}/radare2_${R2_VERSION}_amd64.deb" \
    && apt-get install -y --no-install-recommends /tmp/radare2.deb \
    && rm -f /tmp/radare2.deb \
    && apt-get purge -y curl \
    && apt-get autoremove -y \
    && rm -rf /var/lib/apt/lists/*

# pycryptodome covers the common malware crypto (AES/DES/RC4/etc.) BINNY's
# extraction scripts reach for; it's pure-enough to install offline-free here.
RUN pip3 install --no-cache-dir --break-system-packages pycryptodome

WORKDIR /work
