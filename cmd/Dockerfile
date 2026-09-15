# syntax=docker/dockerfile:1

# --- build stage -------------------------------------------------------------
FROM golang:1.27-alpine AS builder

# redocly (bundles the OpenAPI spec) is a Node tool; ogen comes from go.mod's
# `tool` directive. Both are needed by `go generate ./...`.
ARG REDOCLY_VERSION=2.53.1
RUN apk add --no-cache nodejs npm \
    && npm install -g --no-fund --no-audit "@redocly/cli@${REDOCLY_VERSION}" \
    && npm cache clean --force

WORKDIR /src

# Dependencies first so they stay cached across source changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# Bundles internal/api/bundled/*.yaml and regenerates internal/api/generated/.
# Both are git-ignored and excluded via .dockerignore, so this always builds
# from the spec rather than whatever happens to be on the host.
RUN --mount=type=cache,target=/go/pkg/mod go generate ./...

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd

# --- runtime stage -----------------------------------------------------------
FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 app

COPY --from=builder /out/api /usr/local/bin/api

USER app
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/api"]
