# Dockerfile for the svpchain lending agent (cmd/svpchain-lending-agent), the
# lending-category slice of the DEX agent (Lendora money market).
#
# The resulting image contains only the agent binary plus ca-certificates.
# Runtime config lives in agent.toml, mounted read-only at
# /etc/svpchain-lending-agent/agent.toml; the optional operator key is mounted
# beside it. Use a DISTINCT operator key per binary — the agent id derives
# from the key and self-registration hashes this binary's own card.
#
# Build from the repo root, from a VENDORED dependency tree (see the
# svpchain-remote-agents Dockerfile header for why):
#   go mod vendor
#   docker build --platform linux/amd64 \
#     -t svpchain-lending-agent:<tag> \
#     -f cmd/svpchain-lending-agent/Dockerfile .

FROM golang:1.25-alpine AS builder

RUN set -eux; apk add --no-cache ca-certificates build-base; apk add git linux-headers bash binutils-gold

WORKDIR /svpchain

COPY . .

ARG VERSION
ARG COMMIT

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    go build \
      -mod=vendor \
      -tags "netgo,muslc" \
      -ldflags "-X github.com/cosmos/cosmos-sdk/version.Name=svpchain \
              -X github.com/cosmos/cosmos-sdk/version.AppName=svpchain-lending-agent \
              -X github.com/cosmos/cosmos-sdk/version.Version=$VERSION \
              -X github.com/cosmos/cosmos-sdk/version.Commit=$COMMIT \
              -X github.com/cosmos/cosmos-sdk/version.BuildTags='netgo,muslc' \
              -w -s -linkmode=external -extldflags '-Wl,-z,muldefs -static'" \
      -trimpath \
      -o /svpchain/build/svpchain-lending-agent \
      ./cmd/svpchain-lending-agent

# --------------------------------------------------------
# Runner
# --------------------------------------------------------

FROM alpine:3.21

RUN apk add --no-cache bash ca-certificates

COPY --from=builder /svpchain/build/svpchain-lending-agent /bin/svpchain-lending-agent

# Default A2A listen port. Override via agent.toml listen_addr.
EXPOSE 8084

ENTRYPOINT ["/bin/svpchain-lending-agent"]
CMD ["-config", "/etc/svpchain-lending-agent/agent.toml"]
