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
| `POST /auth/register` | Create a pending `planner` or `provider` account and email it a code |
| `POST /auth/verify` | Activate a pending account with its six-digit code |
| `POST /auth/resend-verification` | Email a new code to a pending account |
| `POST /auth/login` | Exchange email and password for a session token (active accounts only) |
| `POST /auth/logout` | Revoke the current session (`Authorization: Bearer <token>`) |
| `GET /docs` | Swagger UI |
| `GET /docs/openapi.yaml` | The OpenAPI spec the server was generated from |

## Layout

```
cmd/
  main.go            entrypoint; mounts the ogen server and the docs routes
  Dockerfile         multi-stage build (generates the spec, then compiles)
  seed/              creates test users for every role in a local database
internal/api/
  handler.go         hand-written handlers implementing the ogen interface
  doc.go             Swagger UI and spec-serving handlers (outside ogen)
  middleware/        ogen request logging and shared log helpers
  openapi/
    server.yaml      OpenAPI spec — the source of truth
    paths/           one file per path, $ref'd from server.yaml
    components/      shared schemas and responses, one file each
    generate.go      //go:generate directives (bundle + codegen)
    bundled/         generated: the flattened spec
    generated/       generated: the ogen server
moozo/               domain types: User, Session, VerificationCode, their
                     repositories, and the Mailer interface
  moozotest/         in-memory repository and mailer fakes for tests above them
internal/app/        application layer (CQRS): the HTTP handler calls a
  command/           command (changes state) or a query (reads state), which
  query/             holds the use case's logic and calls the repositories
internal/mail/       Mailer implementations: the app's emails sent through
                     pkg/brevo, and a logger for development
internal/mongodb/     MongoDB repositories
  schemas/           one JSON validator + indexes file per collection
pkg/brevo/           app-agnostic Brevo API client (email; SMS later)
```

`bundled/` and `generated/` are git-ignored and rebuilt from `openapi/server.yaml`.

## Code generation

Adding or changing an endpoint means editing `internal/api/openapi/server.yaml`, then:

```sh
npm ci             # once, installs the pinned redocly
go generate ./...
```

That bundles the spec into `internal/api/openapi/bundled` and regenerates
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

They are registered only when `PRODUCTION` is false, so in production the
routes cease to exist rather than returning 404 from a registered path.

## Configuration

Loaded from the environment at startup (`internal/config`), all names uppercase.
See `.env.example`.

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | |
| `PRODUCTION` | `false` | When true, `/docs` and `/docs/openapi.yaml` are not registered |
| `MONGODB_URI` | `mongodb://localhost:27017` | No auth — local development only |
| `MONGODB_DATABASE` | `moozo` | |
| `BREVO_API_KEY` | | Brevo API key for transactional email. Unset: verification codes are logged instead. Required when `PRODUCTION` is true |
| `MAIL_FROM_EMAIL` | | Sender address; must be verified in Brevo. Required with `BREVO_API_KEY` |
| `MAIL_FROM_NAME` | `Moozo` | Sender name |

The server connects and pings MongoDB at startup and refuses to start if it
can't.

## Users

Accounts live in the `users` collection, whose schema is
`internal/mongodb/schemas/users.json`: a `$jsonSchema` validator (strict, rejects
unknown fields) and a case-insensitive unique index on `email` (collation
strength 2).

Every `*.json` in that directory is embedded in the binary and describes the
collection it is named after. `mongodb.NewRepository` applies them all at
startup: it creates missing collections, re-applies validators to existing
ones with `collMod`, and rebuilds an index whose options changed, so schema
edits roll out on deploy. Indexes removed from a file are not dropped — that
needs a manual `dropIndex`.

`moozo.User` has no storage tags; `internal/mongodb` maps it to a private
`userDocument`. Tests catch the copies drifting apart:
`internal/mongodb/schema_test.go` (each schema file vs its document type, and
the role and status enums vs `moozo.Roles` / `moozo.Statuses`) and
`TestSpecEnumsMatchDomain` in `internal/api` (the spec's enums vs the domain's).

| Field | Type | |
|---|---|---|
| `_id` | binData (UUID v7, subtype 4) | time-ordered, generated by the register command |
| `email` | string | lowercased and trimmed, unique ignoring case |
| `password_hash` | string | bcrypt; never returned by the API |
| `role` | `planner` \| `provider` \| `admin` | |
| `status` | `pending` \| `active` | see [Email verification](#email-verification) |
| `created_at`, `updated_at` | date | |

`POST /auth/register` only accepts `planner` and `provider`: the spec's
`RegisterRole` enum leaves out `admin`, so ogen rejects it with a 400 before the
handler runs, and the register command checks again against `moozo.SelfServiceRoles`.
Admins must be created some other way — locally, `go run ./cmd/seed` (see
[Seeding test users](#seeding-test-users)). A taken email returns 409.

## Email verification

`POST /auth/register` creates the account as `pending` and emails it a
six-digit code. A pending account cannot log in: the right password gets a 403
(a wrong one still gets the usual 401, so the status is not revealed).
`POST /auth/verify` with the email and code makes it `active`.

- A code is valid for 15 minutes (`command.VerificationCodeLifetime`) and for
  5 attempts (`command.MaxVerificationAttempts`); after either it is deleted
  and the user must ask for a new one. Each attempt is counted atomically
  before the comparison, so parallel guesses cannot exceed the limit.
- Codes live in the `verification_codes` collection, keyed by user ID, so a
  user has at most one: sending a new code replaces the old. Only the code's
  SHA-256 is stored, and a TTL index on `expires_at` cleans up expired ones.
- `POST /auth/resend-verification` always answers 202, whether the email is
  unknown, already active or pending, and sends at most one code a minute
  (`command.ResendCooldown`).
- A wrong code, an expired one and an unknown email all get the same 400.
  Verifying an already active account succeeds, so a retried request is
  harmless.
- If the email cannot be sent at sign-up, the account is still created
  (201) and the failure is logged; the user can resend.
- Seeded accounts are created `active`.

Emails go through [Brevo](https://developers.brevo.com)'s transactional API,
chosen because it also does SMS. `pkg/brevo` is a generic client for its API;
`internal/mail` writes the application's emails and sends them with it. Without `BREVO_API_KEY`
the server logs each code instead — look for `verification code` in the API
logs. Production refuses to start without a key.

> Users created before verification existed have no `status` and cannot log
> in. Drop the local database (`docker compose down -v`, or `db.dropDatabase()`
> in `mongosh`) and rerun the seed.

## Sessions

`POST /auth/login` returns an opaque, random 256-bit token, valid for seven
days (`command.SessionLifetime`). Clients send it as
`Authorization: Bearer <token>` on operations that declare `bearerAuth` in the
spec; ogen calls `Handler.HandleBearerAuth`, which runs the `Authenticate`
query and puts the `moozo.Session` in the request context.

- Only the token's SHA-256 is stored, as the `_id` of the `sessions`
  collection, so a database leak does not hand out working tokens.
- `POST /auth/logout` deletes the session: revocation is immediate.
- The session stores the user's role at login, so authorization needs no user
  lookup. Anything that changes a user's role must also delete their
  sessions, or the old role lasts until they expire.
- A TTL index on `expires_at` lets MongoDB delete expired sessions; it runs
  about once a minute, so `Authenticate` also checks the expiry itself.
- A wrong password and an unknown email get the same 401, and both cost one
  bcrypt comparison, so neither the response nor its timing reveals which
  emails have accounts.
- ogen answers every security-handler error with 401; `statusCode` keeps 401
  only for a missing or invalid token, so a database outage during
  authentication is a 500. These errors bypass `LoggingMiddleware`, so
  `Handler.NewError` logs them.

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
- **test** — `go build`, `go test -race`, and coverage (gated on lint passing),
  with a `mongo:7` service container for the MongoDB integration tests

Coverage excludes `internal/api/openapi/generated`; including ogen's output drags the
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

The repository and schema tests in `internal/mongodb` need a real MongoDB. They
skip unless `MONGODB_TEST_URI` is set (and fail instead when `CI` is set). Each
test creates and drops its own `test_<id>` database, so any server will do,
including the compose one:

```sh
docker compose up -d mongo
MONGODB_TEST_URI=mongodb://localhost:27017 go test ./...
```

### Seeding test users

`cmd/seed` creates three accounts per role, admins included, through
`command.CreateUser` (the role-unrestricted core of `RegisterUser`). It reads
`MONGODB_URI` and `MONGODB_DATABASE` like the server, refuses to run when
`PRODUCTION=true`, and skips accounts that already exist, so it is safe to
rerun:

```sh
docker compose up -d mongo
go run ./cmd/seed
```

Every seeded account has the password **`password123`** — local testing only.

| Role | Emails |
|---|---|
| `planner` | `planner1@moozo.test`, `planner2@moozo.test`, `planner3@moozo.test` |
| `provider` | `provider1@moozo.test`, `provider2@moozo.test`, `provider3@moozo.test` |
| `admin` | `admin1@moozo.test`, `admin2@moozo.test`, `admin3@moozo.test` |

```sh
curl -s localhost:8080/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"admin1@moozo.test","password":"password123"}'
```

Linting locally, matching CI:

```sh
docker run --rm -v "$PWD":/app -w /app \
  golangci/golangci-lint:v2.13.2 golangci-lint run
```
