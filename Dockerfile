# syntax=docker/dockerfile:1@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89
ARG ALPINE_REF=alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
ARG UBUNTU_REF=ubuntu:24.04@sha256:4fbb8e6a8395de5a7550b33509421a2bafbc0aab6c06ba2cef9ebffbc7092d90
ARG NODE_REF=node:18@sha256:c6ae79e38498325db67193d391e6ec1d224d96c693a8a4d943498556716d3783
ARG CONTAINER_TEMPLATE_REF=ghcr.io/spr-networks/container_template@sha256:869ada7b121e9a0c552674042d32e801da3c4d04145638d9e722918c6377e65f
ARG SOURCE_DATE_EPOCH

FROM ${ALPINE_REF} AS cacerts

FROM ${UBUNTU_REF} AS builder
ENV DEBIAN_FRONTEND=noninteractive
ARG UBUNTU_SNAPSHOT=20260601T000000Z
ARG GO_VERSION=1.25.12
ARG GO_SHA256_AMD64=234828b7a89e0e303d2556310ee549fbcf253d28de937bac3da13d6294262ac1
ARG GO_SHA256_ARM64=8b5884aef89600aef5b0b051fb971f11f49bb996521e911f30f02a66884f7bd2
ARG SMP_VERSION=v6.5.0
ARG SMP_SHA256_AMD64=0ec0984a9f15d8a140c96e0c84948e37f581ed0a0140e6fc1d5677dcc9e5144c
ARG SMP_SHA256_ARM64=f7c27a765dc34cb9c9ee9e220f2cfb3ea4005f24a2991a0be16b676620e13fa6
ARG TARGETARCH
COPY --from=cacerts /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN set -eux; \
    printf 'Types: deb\nURIs: https://snapshot.ubuntu.com/ubuntu/%s\nSuites: noble noble-updates noble-security\nComponents: main restricted universe multiverse\nSigned-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg\n' "${UBUNTU_SNAPSHOT}" > /etc/apt/sources.list.d/ubuntu.sources; \
    printf 'APT::Install-Recommends "false";\nAcquire::Check-Valid-Until "false";\n' > /etc/apt/apt.conf.d/99reproducible
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git wget && rm -rf /var/lib/apt/lists/* /var/log/* /var/cache/ldconfig/aux-cache
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) GO_SHA256="${GO_SHA256_AMD64}";; \
      arm64) GO_SHA256="${GO_SHA256_ARM64}";; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1;; \
    esac; \
    wget -q "https://dl.google.com/go/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz"; \
    echo "${GO_SHA256}  go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" | sha256sum -c -; \
    tar -C /usr/local -xzf "go${GO_VERSION}.linux-${TARGETARCH}.tar.gz"; \
    rm "go${GO_VERSION}.linux-${TARGETARCH}.tar.gz"
ENV PATH="/usr/local/go/bin:${PATH}" GOTOOLCHAIN=local
# smp-server: official upstream release binary (Haskell; building from source
# is out of scope), pinned by version + sha256 of each architecture's asset.
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) SMP_ASSET="smp-server-ubuntu-24_04-x86-64"; SMP_SHA256="${SMP_SHA256_AMD64}";; \
      arm64) SMP_ASSET="smp-server-ubuntu-24_04-aarch64"; SMP_SHA256="${SMP_SHA256_ARM64}";; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1;; \
    esac; \
    wget -q "https://github.com/simplex-chat/simplexmq/releases/download/${SMP_VERSION}/${SMP_ASSET}"; \
    echo "${SMP_SHA256}  ${SMP_ASSET}" | sha256sum -c -; \
    install -m 0755 "${SMP_ASSET}" /smp-server; \
    rm "${SMP_ASSET}"
WORKDIR /code
COPY code/ /code/
RUN --mount=type=tmpfs,target=/root/go/ go build -trimpath -ldflags "-s -w -X main.PinnedSMPVersion=${SMP_VERSION}" -o /simplex_plugin /code/

FROM ${NODE_REF} AS builder-ui
WORKDIR /app
COPY frontend ./
RUN --mount=type=tmpfs,target=/root/.cache \
    --mount=type=tmpfs,target=/app/node_modules \
    yarn install --frozen-lockfile --network-timeout 86400000 && yarn run bundle

FROM ${CONTAINER_TEMPLATE_REF}
ENV DEBIAN_FRONTEND=noninteractive
ARG UBUNTU_SNAPSHOT=20260601T000000Z
# Shared libraries the upstream smp-server binary links against
# (libcrypto.so.3, libgmp.so.10, libz.so.1), from the same dated snapshot.
RUN set -eux; \
    printf 'Types: deb\nURIs: https://snapshot.ubuntu.com/ubuntu/%s\nSuites: noble noble-updates noble-security\nComponents: main restricted universe multiverse\nSigned-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg\n' "${UBUNTU_SNAPSHOT}" > /etc/apt/sources.list.d/ubuntu.sources; \
    printf 'APT::Install-Recommends "false";\nAcquire::Check-Valid-Until "false";\n' > /etc/apt/apt.conf.d/99reproducible; \
    apt-get update && apt-get install -y --no-install-recommends libssl3t64 libgmp10 zlib1g && rm -rf /var/lib/apt/lists/* /var/log/* /var/cache/ldconfig/aux-cache
COPY scripts /scripts/
COPY --from=builder /smp-server /usr/local/bin/smp-server
COPY --from=builder /simplex_plugin /
COPY --from=builder-ui /app/build/ /ui/

ENTRYPOINT ["/scripts/startup.sh"]
