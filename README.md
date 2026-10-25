# Backend Golang Coding Test

| Part | Deliverable |
| --- | --- |
| 1. User Management API | Code in this repository; documented below |
| 2. Lottery Search System | Design proposal: [docs/lottery-search-design.md](docs/lottery-search-design.md) |

## Part 1: User Management API

A RESTful (and gRPC) user management service in Go, backed by MongoDB, with JWT (HS256) authentication. It uses a hexagonal (ports and adapters) structure.

## Contents

- [Features](#features)
- [Project structure](#project-structure)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [JWT guide](#jwt-guide)
- [REST API](#rest-api)
- [gRPC API](#grpc-api)
- [Testing](#testing)
- [Design decisions and assumptions](#design-decisions-and-assumptions)

## Features

| Requirement | Where |
| --- | --- |
| User model (ID, Name, Email unique, hashed Password, CreatedAt) | `internal/core/domain/user.go`, `internal/adapter/outbound/mongodb` |
| Register / login returning a JWT | `internal/core/service/auth_service.go`, `POST /api/v1/auth/*` |
| JWT middleware, HS256 with secret key | `internal/adapter/inbound/httpapi/middleware.go`, `internal/adapter/outbound/security/jwt.go` |
| Create, get, list, update (name/email), delete | `internal/core/service/user_service.go`, `/api/v1/users` |
| Official MongoDB Go driver (v2) | `internal/adapter/outbound/mongodb` |
| Logging middleware (method, path, status, duration) | `httpapi.Logging` |
| Goroutine that logs the user count every 10s | `internal/adapter/inbound/scheduler/user_count.go` |
| Unit tests with `testing` and mocked MongoDB | `*_test.go`, `internal/mocks` |
| **Bonus:** Docker + docker-compose | `Dockerfile`, `docker-compose.yml` |
| **Bonus:** interface abstraction of MongoDB | `port.UserRepository` |
| **Bonus:** input validation | `internal/core/domain/user.go` |
| **Bonus:** graceful shutdown with `context.Context` | `cmd/api/main.go` |
| **Bonus:** gRPC `CreateUser` / `GetUser`, secured with token metadata | `api/proto/user/v1/user.proto`, `internal/adapter/inbound/grpcapi` |
| **Bonus:** hexagonal architecture | see below |

## Project structure

```text
.
├── cmd/api/main.go                    # composition root: wiring, servers, graceful shutdown
├── api/proto/user/v1/user.proto       # gRPC contract
├── gen/user/v1/                       # generated protobuf / gRPC code
└── internal/
    ├── core/                          # ── business logic, no framework/driver imports
    │   ├── domain/                    # User entity, validation rules, domain errors
    │   ├── port/                      # interfaces: driven (repository, hasher, tokens) and driving (use cases)
    │   └── service/                   # use cases: UserService, AuthService
    ├── adapter/
    │   ├── inbound/                   # ── driving adapters (call into the core)
    │   │   ├── httpapi/               # REST handlers, JWT + logging + recover middleware
    │   │   ├── grpcapi/               # gRPC server, auth + logging interceptors
    │   │   └── scheduler/             # background user-count reporter
    │   └── outbound/                  # ── driven adapters (called by the core)
    │       ├── mongodb/               # UserRepository on the official driver
    │       └── security/              # bcrypt hasher, HS256 JWT service
    ├── config/                        # environment configuration
    └── mocks/                         # hand-written port mocks + in-memory repository
```

Dependencies point inward only: `adapter → core/port ← core/service → core/domain`. The core never imports MongoDB, `net/http`, gRPC or JWT libraries, so any adapter can be swapped or mocked.

## Quick start

### Option A: Docker Compose (API + MongoDB)

```bash
cp .env.example .env          # then set JWT_SECRET to a random value (≥ 32 bytes)
docker compose up --build
```

- REST: `http://localhost:8080`
- gRPC: `localhost:9090`
- MongoDB: `localhost:27017`

### Option B: run locally

Requires Go 1.27+ and a running MongoDB.

```bash
docker run -d --name mongo -p 27017:27017 mongo:8.0    # or any MongoDB instance

export JWT_SECRET="$(openssl rand -base64 48)"
export MONGO_URI="mongodb://localhost:27017"
go run ./cmd/api
```

PowerShell:

```powershell
$env:JWT_SECRET = "replace-with-a-random-string-of-at-least-32-bytes"
$env:MONGO_URI  = "mongodb://localhost:27017"
go run ./cmd/api
```

Every 10 seconds the log shows:

```json
{"time":"2026-10-25T08:00:10Z","level":"INFO","msg":"total users","count":2}
```

Press `Ctrl+C` (SIGINT) or send SIGTERM to shut down gracefully. In-flight HTTP and gRPC requests are drained, the background job stops, and the MongoDB client disconnects.

## Configuration

All settings come from environment variables.

| Variable | Default | Description |
| --- | --- | --- |
| `JWT_SECRET` | **required** | HMAC key for HS256. Must be at least 32 bytes. |
| `JWT_TTL` | `24h` | Token lifetime (Go duration). |
| `JWT_ISSUER` | `user-management-api` | `iss` claim; verified on every request. |
| `MONGO_URI` | `mongodb://localhost:27017` | MongoDB connection string. |
| `MONGO_DATABASE` | `user_management` | Database name. |
| `HTTP_ADDR` | `:8080` | REST listen address. |
| `GRPC_ADDR` | `:9090` | gRPC listen address. |
| `BCRYPT_COST` | `12` | bcrypt work factor. |
| `USER_COUNT_INTERVAL` | `10s` | Interval of the user-count log job. |
| `SHUTDOWN_TIMEOUT` | `15s` | Maximum time to drain requests on shutdown. |

## JWT guide

### 1. Get a token

Register (once), then log in:

```bash
curl -s -X POST localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"name":"Alice","email":"alice@example.com","password":"password123"}'

curl -s -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123"}'
```

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6ImFsaWNlQGV4YW1wbGUuY29tIiwiaXNzIjoidXNlci1tYW5hZ2VtZW50LWFwaSIsInN1YiI6IjY3MWI...",
  "token_type": "Bearer",
  "expires_at": "2026-10-26T08:00:00Z",
  "user": {
    "id": "671b1f0e8c3a4d2b9e5f7a10",
    "name": "Alice",
    "email": "alice@example.com",
    "created_at": "2026-10-25T08:00:00Z"
  }
}
```

### 2. Use the token

Send it in the `Authorization` header on every `/api/v1/users` request:

```bash
TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"password123"}' | jq -r .access_token)

curl -s localhost:8080/api/v1/users -H "Authorization: Bearer $TOKEN"
```

For gRPC, send the same value as the `authorization` metadata key.

### 3. What is inside the token

Header `{"alg":"HS256","typ":"JWT"}`; payload:

| Claim | Meaning |
| --- | --- |
| `sub` | User ID |
| `email` | User email at login time |
| `iss` | `JWT_ISSUER` |
| `iat`, `nbf` | Issued-at / not-before |
| `exp` | Expiry (`iat + JWT_TTL`) |

### 4. How tokens are validated

The middleware rejects a request with `401` and `WWW-Authenticate: Bearer` when:

- the header is missing or not `Bearer <token>`
- the signature does not match `JWT_SECRET`
- `alg` is anything other than `HS256` (blocks `alg: none` and algorithm-confusion attacks)
- `exp` is missing or in the past, `iat` is in the future, or `iss` is wrong
- the user in `sub` no longer exists (deleting a user revokes their tokens immediately)

## REST API

Base URL: `http://localhost:8080`. All bodies are JSON. Unknown JSON fields are rejected, and bodies are limited to 1 MiB.

| Method | Path | Auth | Description | Success |
| --- | --- | --- | --- | --- |
| `GET` | `/healthz` | — | Liveness check | `200` |
| `POST` | `/api/v1/auth/register` | — | Register a user | `201` |
| `POST` | `/api/v1/auth/login` | — | Get a JWT | `200` |
| `POST` | `/api/v1/users` | JWT | Create a user | `201` |
| `GET` | `/api/v1/users` | JWT | List all users | `200` |
| `GET` | `/api/v1/users/{id}` | JWT | Get a user by ID | `200` |
| `PATCH` | `/api/v1/users/{id}` | JWT | Update name and/or email | `200` |
| `DELETE` | `/api/v1/users/{id}` | JWT | Delete a user | `204` |

### Validation rules

| Field | Rule |
| --- | --- |
| `name` | Required, trimmed, 1–100 characters |
| `email` | Required, valid address (`local@domain.tld`), trimmed and lower-cased, unique |
| `password` | Required, 8 characters to 72 bytes (bcrypt limit) |

### Error format

```json
{
  "error": {
    "code": "validation_error",
    "message": "request validation failed",
    "fields": { "email": "must be a valid email address" }
  }
}
```

| Status | `code` | When |
| --- | --- | --- |
| `400` | `bad_request` | Malformed / empty / oversized JSON, unknown field |
| `400` | `validation_error` | Field validation failed (`fields` lists each problem) |
| `401` | `unauthorized` | Missing, invalid or expired token |
| `401` | `invalid_credentials` | Wrong email or password at login |
| `404` | `not_found` | User does not exist (or ID is malformed) |
| `409` | `email_exists` | Email already used by another user |
| `500` | `internal_error` | Unexpected error (details are logged, never returned) |

### Samples

**Register**

```http
POST /api/v1/auth/register
Content-Type: application/json

{"name": "Alice", "email": "Alice@Example.com", "password": "password123"}
```

```http
HTTP/1.1 201 Created
Location: /api/v1/users/671b1f0e8c3a4d2b9e5f7a10

{"id":"671b1f0e8c3a4d2b9e5f7a10","name":"Alice","email":"alice@example.com","created_at":"2026-10-25T08:00:00Z"}
```

**Register with a taken email**

```http
HTTP/1.1 409 Conflict

{"error":{"code":"email_exists","message":"email already exists"}}
```

**Login with a wrong password**

```http
HTTP/1.1 401 Unauthorized

{"error":{"code":"invalid_credentials","message":"invalid email or password"}}
```

**Create a user**

```bash
curl -s -X POST localhost:8080/api/v1/users \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Bob","email":"bob@example.com","password":"password123"}'
```

```json
{"id":"671b1f2a8c3a4d2b9e5f7a11","name":"Bob","email":"bob@example.com","created_at":"2026-10-25T08:01:00Z"}
```

**List users**

```bash
curl -s localhost:8080/api/v1/users -H "Authorization: Bearer $TOKEN"
```

```json
{
  "data": [
    {"id":"671b1f0e8c3a4d2b9e5f7a10","name":"Alice","email":"alice@example.com","created_at":"2026-10-25T08:00:00Z"},
    {"id":"671b1f2a8c3a4d2b9e5f7a11","name":"Bob","email":"bob@example.com","created_at":"2026-10-25T08:01:00Z"}
  ],
  "total": 2
}
```

**Get a user**

```bash
curl -s localhost:8080/api/v1/users/671b1f2a8c3a4d2b9e5f7a11 -H "Authorization: Bearer $TOKEN"
```

```json
{"id":"671b1f2a8c3a4d2b9e5f7a11","name":"Bob","email":"bob@example.com","created_at":"2026-10-25T08:01:00Z"}
```

**Update a user** (send `name`, `email` or both)

```bash
curl -s -X PATCH localhost:8080/api/v1/users/671b1f2a8c3a4d2b9e5f7a11 \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Robert"}'
```

```json
{"id":"671b1f2a8c3a4d2b9e5f7a11","name":"Robert","email":"bob@example.com","created_at":"2026-10-25T08:01:00Z"}
```

**Delete a user**

```bash
curl -i -X DELETE localhost:8080/api/v1/users/671b1f2a8c3a4d2b9e5f7a11 -H "Authorization: Bearer $TOKEN"
```

```http
HTTP/1.1 204 No Content
```

**Request without a token**

```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer

{"error":{"code":"unauthorized","message":"missing or malformed Authorization header"}}
```

### Request log

Each request produces one structured log line:

```json
{"time":"2026-10-25T08:01:00Z","level":"INFO","msg":"http request","method":"POST","path":"/api/v1/users","status":201,"duration":71234500}
```

`duration` is in nanoseconds (slog's JSON encoding of `time.Duration`).

## gRPC API

Service `user.v1.UserService` on `:9090` (`api/proto/user/v1/user.proto`):

- `CreateUser(name, email, password) → User`
- `GetUser(id) → User`

Both RPCs require `authorization: Bearer <jwt>` metadata. Errors map to gRPC codes: `InvalidArgument`, `NotFound`, `AlreadyExists`, `Unauthenticated` and `Internal`.

Example with [grpcurl](https://github.com/fullstorydev/grpcurl):

```bash
grpcurl -plaintext -import-path api/proto -proto user/v1/user.proto \
  -H "authorization: Bearer $TOKEN" \
  -d '{"name":"Carol","email":"carol@example.com","password":"password123"}' \
  localhost:9090 user.v1.UserService/CreateUser

grpcurl -plaintext -import-path api/proto -proto user/v1/user.proto \
  -H "authorization: Bearer $TOKEN" \
  -d '{"id":"671b1f0e8c3a4d2b9e5f7a10"}' \
  localhost:9090 user.v1.UserService/GetUser
```

```json
{
  "user": {
    "id": "671b1f0e8c3a4d2b9e5f7a10",
    "name": "Alice",
    "email": "alice@example.com",
    "createdAt": "2026-10-25T08:00:00Z"
  }
}
```

To regenerate code after editing the proto, run `make proto` (requires `protoc`, `protoc-gen-go` and `protoc-gen-go-grpc`).

## Testing

```bash
go test ./...              # unit tests
go test -race -cover ./... # with race detector (needs cgo) and coverage
```

| Package | What is tested | How MongoDB is replaced |
| --- | --- | --- |
| `core/domain` | Validation and normalization rules | — |
| `core/service` | Use cases: hashing, normalization, error paths, timing-safe login, token revocation on delete | `mocks.UserRepository` (function-field mock that panics on unexpected calls) |
| `adapter/outbound/security` | JWT round trip; rejects tampered, wrong-key, `alg:none`, HS512, expired, no-`exp`, wrong-issuer and future-`iat` tokens; bcrypt | — |
| `adapter/inbound/httpapi` | Full register → login → CRUD flow over real HTTP; status codes; middleware (auth, logging, recover); no error-detail leaks | `mocks.MemoryUserRepository` (in-memory fake with unique-email semantics) |
| `adapter/inbound/grpcapi` | Auth interceptor, CreateUser/GetUser, error-code mapping via `bufconn` | `mocks.MemoryUserRepository` |
| `adapter/inbound/scheduler` | Periodic count logging, error logging, stop on cancel | fake counter |
| `config` | Defaults, overrides, invalid values | — |
| `adapter/outbound/mongodb` | Integration test against real MongoDB (CRUD, unique index, ObjectID handling) | real MongoDB; skipped unless `MONGO_TEST_URI` is set |

Run the MongoDB integration test:

```bash
docker compose up -d mongo
MONGO_TEST_URI=mongodb://localhost:27017 go test ./internal/adapter/outbound/mongodb/ -v
```

### Manual end-to-end verification

The compiled server was also run against a real MongoDB 9.0 instance and exercised with `curl` and `grpcurl`. The following behaviours were confirmed:

- Every REST endpoint returned the documented status code and body, including `409` on duplicate email (register and update), `400` on validation errors and unknown fields, `404` on malformed and unknown IDs, and `401` without a token.
- gRPC `CreateUser` / `GetUser` work with a REST-issued token, and calls without one are rejected with `Unauthenticated`.
- The user-count goroutine logged `total users` every 10 seconds.
- After a user deleted their own account, their token was rejected with `401`.
- On interrupt, an in-flight request still completed with `201`. The reporter then stopped, both servers drained, and the process exited with code 0.

## Design decisions and assumptions

**Architecture**

- **Hexagonal layout.** The core (`domain`, `port`, `service`) has no dependency on MongoDB, HTTP, gRPC or JWT libraries. REST, gRPC and the scheduler are all driving adapters on the same `port.UserService`, so business rules live in one place.
- **Separate persistence model.** `mongodb.userDocument` holds the BSON tags and `ObjectID`; the domain `User` uses a plain string ID. Storage details do not leak into the core.
- **Standard library HTTP.** Go 1.22+ `ServeMux` supports method and path-parameter routing, so no web framework is needed. Logging uses `log/slog` with JSON output.
- **Hand-written mocks.** These avoid a code-generation dependency. Function-field mocks panic on unexpected calls, which catches accidental dependency use (for example, a repository call on invalid input).

**Security**

- **Passwords** are hashed with bcrypt (cost 12). Passwords over 72 bytes are rejected rather than silently truncated by bcrypt.
- **JWT**: HS256 only, `exp` required, issuer checked, secret at least 32 bytes and required at startup (there is no default secret).
- **Revocation on delete**: the auth middleware checks that the token's user still exists. This costs one indexed lookup by `_id` per request and means deleted users cannot keep using old tokens.
- **No user enumeration**: login returns the same error for an unknown email and a wrong password. It also runs a bcrypt comparison against a dummy hash for unknown emails, so response timing does not reveal which emails exist.
- **Hardening**: internal errors are logged but never returned to the client. The server sets read/write timeouts, limits bodies to 1 MiB, rejects unknown JSON fields, and runs the Docker image as non-root on distroless.

**Data**

- **Email uniqueness** is enforced by a unique MongoDB index (`uniq_email`) created at startup. The duplicate-key error is mapped to `409`. This is race-free, unlike a check-then-insert.
- **Emails are lower-cased and trimmed** before storing and lookup, so `Alice@Example.com` and `alice@example.com` are the same account.
- **Malformed IDs** (not a 24-hex ObjectID) return `404` rather than `400`, because they cannot identify any user.
- **`created_at`** is set by the service in UTC and truncated to milliseconds to match MongoDB's precision.

**Scope assumptions**

- **`POST /auth/register` and `POST /users`** both create a user. The first is public self-sign-up; the second is an authenticated "admin creates user" operation. Both share the same use case.
- **Authorization**: any authenticated user may manage any user. The spec only asks for JWT protection and has no roles. Adding RBAC would mean adding a `role` claim and a check in the service layer.
- **Password changes** are out of scope. `PATCH` accepts only `name` and `email`, as the spec states; any other field is rejected with `400`.
- **No pagination** on `GET /users`, because the spec says "list all". For large datasets, `limit`/cursor parameters would be added at the repository port.
- **The user-count job** runs as a goroutine with a `time.Ticker`. It stops when the shutdown context is cancelled, and each count has a timeout so a slow database cannot pile up ticks.
- **gRPC** exposes only `CreateUser` and `GetUser`, as specified, both protected by token metadata. Tokens are obtained from the REST login endpoint.
