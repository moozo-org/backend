# syntax=docker/dockerfile:1

# --- spec bundle stage -------------------------------------------------------
FROM node:22-alpine AS spec

WORKDIR /src

# redocly is pinned in package.json / package-lock.json.
COPY package.json package-lock.json ./
RUN npm ci --no-fund --no-audit

COPY internal/api/ ./internal/api/
WORKDIR /src/internal/api
RUN npx --no-install redocly bundle server.yaml -o /out/server.yaml

# --- build stage -------------------------------------------------------------
FROM golang:1.27.1-alpine AS builder

WORKDIR /src

# Dependencies first so they stay cached across source changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# internal/api/{bundled,generated} are git-ignored and excluded via
# .dockerignore, so this always builds from the spec rather than whatever
# happens to be on the host.
COPY --from=spec /out/server.yaml internal/api/bundled/server.yaml

# The spec is already bundled above, so only the ogen directive runs here. Its
# flags still live in internal/api/generate.go, the single source of truth.
RUN --mount=type=cache,target=/go/pkg/mod go generate -run 'go tool ogen' ./...

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
