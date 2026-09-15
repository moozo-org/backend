# backend

Moozo API — a Go HTTP service whose handlers are generated from an OpenAPI
spec, backed by MongoDB.

## Stack

| | |
|---|---|
| Language | Go 1.27 (module `moozo`) |
| HTTP server | [ogen](https://github.com/ogen-go/ogen), generated from OpenAPI 3.1 |
| Spec bundler | [redocly](https://redocly.com/docs/cli) (Node, pinned in `package.json`) |
| Database | MongoDB |

## Quick start

```sh
docker compose up --build
```

Brings up the API on `:8080` and MongoDB on `:27017`.

| Endpoint | Description |
|---|---|
| `GET /hello` | Example endpoint |
| `GET /docs` | Swagger UI |
| `GET /docs/openapi.yaml` | The OpenAPI spec the server was generated from |

## Layout

```
cmd/
  main.go            entrypoint; mounts the ogen server and the docs routes
  Dockerfile         multi-stage build (generates the spec, then compiles)
internal/api/
  server.yaml        OpenAPI spec — the source of truth
  generate.go        //go:generate directives (bundle + codegen)
  handler.go         hand-written handlers implementing the ogen interface
  doc.go             Swagger UI and spec-serving handlers (outside ogen)
  bundled/           generated: the flattened spec
  generated/         generated: the ogen server
```

`bundled/` and `generated/` are git-ignored and rebuilt from `server.yaml`.

## Code generation

Adding or changing an endpoint means editing `internal/api/server.yaml`, then:

```sh
npm ci             # once, installs the pinned redocly
go generate ./...
```

That bundles the spec into `internal/api/bundled/server.yaml` and regenerates
the ogen server from it. Implement any new operation on `Handler` in
`handler.go` — the build fails until every operation in the spec has a method,
which is the intended feedback loop.

A global `npm i -g @redocly/cli` works too; `npx --no-install` finds it on
`PATH`. Either way the version must match `package.json`, and `npm ci` is what
guarantees that in Docker and CI.

### Docs endpoints

`/docs` and `/docs/openapi.yaml` are deliberately **not** in the OpenAPI spec.
They're plain `net/http` handlers mounted on a `ServeMux` in `main.go`, ahead of
the ogen server. They serve static content, so codegen bought nothing, and
keeping them out means the published spec describes only real API surface.

This also makes them trivial to gate later — wrap the two `HandleFunc` calls in
a conditional and they cease to exist when disabled, rather than returning 404
from a route that's still registered.

## Configuration

| Variable | Default (compose) | Notes |
|---|---|---|
| `MONGO_URI` | `mongodb://mongo:27017` | No auth — local development only |
| `MONGO_DB` | `moozo` | |

> **Not yet wired up.** The Go code has no MongoDB driver; these are plumbed
> through compose ready for when the connection lands.

### Local overrides

`compose.override.yaml` is git-ignored and merged over `compose.yaml`
automatically — no `-f` flags. Put machine-specific tweaks there rather than
editing `compose.yaml`.

> **Linux kernel ≥ 6.19:** MongoDB 8 refuses to start
> ([SERVER-121912](https://jira.mongodb.org/browse/SERVER-121912)), and
> `mongo:latest` is 8.x. Symptom is a healthcheck that never passes and
> `dependency failed to start: container moozo-mongo is unhealthy`. Work around
> it with an override until it's fixed upstream:
>
> ```yaml
> services:
>   mongo:
>     image: mongo:7
> ```

## CI

`.github/workflows/ci.yml` runs on pull requests and pushes to `main`:

- **lint** — golangci-lint, max cyclomatic complexity **12**, generated code excluded
- **test** — `go build`, `go test -race`, and coverage (gated on lint passing)

Coverage excludes `internal/api/generated`; including ogen's output drags the
number to near zero regardless of how well the hand-written code is tested. The
profile and an HTML report are uploaded as a build artifact.

Both jobs share `.github/actions/generate`, a composite action that installs
redocly and runs `go generate` — the checked-out tree doesn't compile until it
has, since the generated code isn't committed.

## Local development

```sh
npm ci                    # pinned redocly
go generate ./...         # bundle spec + generate server
go build ./...
go test ./...
docker compose up --build # full stack
```

Linting locally, matching CI:

```sh
docker run --rm -v "$PWD":/app -w /app \
  golangci/golangci-lint:v2.13.2 golangci-lint run
```
