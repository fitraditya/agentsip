# syntax=docker/dockerfile:1
#
# agentsip engine.
#
# Two things about running this container are not optional, same reasoning
# as obrelin's own Dockerfile:
#
#   1. **--network host.** Docker's bridge network NATs, and SIP/SDP and RTP
#      both carry addresses inside their payloads that the NAT does not
#      rewrite. On a bridge the engine advertises an address nothing can
#      reach — the symptom is calls that connect with no audio, not an error
#      anybody notices.
#   2. **Migrations are not applied by this binary.** They travel with the
#      image (copied in below) but an operator runs them explicitly via the
#      golang-migrate CLI — see Makefile's migrate-up/migrate-vectorstore-up
#      targets — before starting a container against a fresh database.
#
# Build:
#   docker build --build-arg VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -t agentsip:local .
# Run:
#   docker run -d --name agentsip --network host --env-file .env --restart unless-stopped agentsip:local

FROM golang:1.26-alpine AS build
WORKDIR /src

# Dependencies first, so a source-only change does not re-download the
# module cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
# CGO_ENABLED=0: nothing in this codebase needs CGo (no audio codec bindings,
# unlike obrelin's Opus encoder) — a static binary needing no libc.
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.buildVersion=${VERSION}" \
    -o /out/engine ./cmd/engine

FROM alpine:3.22
# ca-certificates for TLS to ElevenLabs/the LLM endpoint/webhook receivers;
# tzdata because call timestamps are reasoned about in local time by the
# people reading them.
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 agentsip
COPY --from=build /out/engine /usr/local/bin/engine
# The migrations travel with the binary that expects them — an operator who
# has the container has what they need to apply them.
COPY --from=build /src/db/migrations /opt/agentsip/migrations
COPY --from=build /src/db/migrations_vectorstore /opt/agentsip/migrations_vectorstore

# Non-root. Nothing here binds a privileged port: SIP's 5060 and the admin
# API are both above 1024, and with host networking the container uses the
# host's ports directly.
USER agentsip

# Documentation only — host networking ignores EXPOSE, and the RTP range is
# far too large to list usefully. SIP 5060/udp, the admin API, and
# RTP_PORT_MIN-RTP_PORT_MAX/udp for media.
EXPOSE 5060/udp 8080/tcp

# No shell form: the binary is PID 1 and must receive SIGTERM directly, or
# in-flight calls are dropped without the chance to finish.
ENTRYPOINT ["/usr/local/bin/engine"]
