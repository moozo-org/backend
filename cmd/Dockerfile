# syntax=docker/dockerfile:1

# --- build stage -------------------------------------------------------------
FROM golang:1.27-alpine AS builder

WORKDIR /src

# redocly (bundles the OpenAPI spec) is a Node tool pinned in package.json;
# ogen comes from go.mod's `tool` directive. Both are needed by `go generate`.
COPY package.json package-lock.json ./
RUN apk add --no-cache nodejs npm \
    && npm ci --no-fund --no-audit \
    && npm cache clean --force

# Dependencies first so they stay cached across source changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# Bundles internal/api/bundled/*.yaml and regenerates internal/api/generated/.
# Both are git-ignored and excluded via .dockerignore, so this always builds
# from the spec rather than whatever happens to be on the host.
RUN --mount=type=cache,target=/go/pkg/mod go generate ./...

RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd

# --- runtime stage -----------------------------------------------------------
FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 app

COPY --from=builder /out/api /usr/local/bin/api

USER app
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/api"]
