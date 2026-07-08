# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

A [Traefik](https://traefik.io) **middleware plugin** (Go) that intercepts requests, verifies the `Authorization` bearer token against an external auth service, and — on success — strips the token and forwards user context to downstream services via headers.

**Source of truth is the code, not the docs.** `.traefik.yml` and `doc/*.mermaid` can lag behind `custom_auth.go`. The plugin sets `X-User-Id` (the `id` from the auth-service response) and `X-Request-Id`, and deletes the incoming `Authorization` header — see [doc/consumers.md](doc/consumers.md) for how it's wired into the Duos stack. (History: an `X-User-Info` header — base64 of the full response body — was removed; downstream `core` now resolves the user from the id via the DB.) When code and docs disagree, trust `custom_auth.go`.

## Commands

| Command | What it does |
|---------|--------------|
| `go test -v ./...` | Run all tests (the everyday check; needs only the Go toolchain) |
| `go test -v -run TestCustomAuth ./...` | Run a single test |
| `make test` | Tests with coverage (`go test -v -cover ./...`) |
| `make lint` | `golangci-lint run` — **requires golangci-lint installed** (CI: v1.60.2) |
| `make yaegi_test` | Run the plugin under Traefik's interpreter — **requires yaegi installed** (see below) |
| `make` / `make vendor` | Default = `lint test`; `vendor` = `go mod vendor` |

`golangci-lint` and `yaegi` are **not** assumed present locally — they're installed in CI. Use `go test` for the fast local loop; the full `make` and `make yaegi_test` are what the CI gate actually runs.

## The Yaegi constraint (most important thing to know)

Traefik does **not** compile plugins. It loads them at runtime through [Yaegi](https://github.com/traefik/yaegi), a Go *interpreter*. Consequences that shape every change here:

- **No CGO, no `unsafe`, no build-time codegen.** CI sets `CGO_ENABLED=0`.
- **Avoid third-party dependencies.** This is why `uuid-v7.go` hand-rolls UUID v7 instead of importing `github.com/google/uuid` (the orphaned `go.sum` entry is a leftover — the code imports nothing beyond stdlib). Prefer stdlib; a new dep must survive interpretation *and* be vendored.
- **`make yaegi_test` is the real gate.** Code can pass `go test` yet fail under Yaegi. If you touch plugin logic, it must still interpret. `.traefik.yml`'s `testData` block is the config Yaegi runs with.

## Traefik plugin contract

Traefik discovers the plugin by **fixed symbol names** — do not rename these:

- `CreateConfig() *Config` — returns defaults (`Production: false`).
- `New(ctx, next http.Handler, config *Config, name string) (http.Handler, error)` — constructor; validates `AuthURL`.
- `ServeHTTP(rw, req)` on the returned handler — the middleware body.
- `.traefik.yml` is the **plugin manifest** (displayName, `type: middleware`, import path, testData) — Traefik's catalog reads it.

The module path `github.com/lifter-ai/auth-token-exchange-plugin` is coupled across **three** places: `go.mod`, `.traefik.yml` (`import:`), and the test's import. Change one → change all three. Package name is `auth_token_exchange_plugin` (underscored).

## Request flow (`ServeHTTP`)

```
Authorization missing ────────────────────────────────► 401 "Missing Authorization header"
!production && token == "test-token" ─────────────────► 200 OK, request NOT forwarded (integration test shortcut)
otherwise:
  set X-Request-Id (UUID v7)
  GET authURL with the original Authorization header    (up to 3 attempts, exponential backoff 100ms×2ⁿ + jitter)
    unreachable after retries ────────────────────────► 500
    resp 401 ─────────────────────────────────────────► 401 "Invalid token"
    resp non-200 ─────────────────────────────────────► passthrough that status
    resp 200 → decode JSON body:
      set X-User-Id = body["id"]
      DELETE Authorization header
      forward to next handler
```

The "token exchange" *is* those forwarded headers — downstream services read `X-User-Id` for user context and never see the original credential. `production=false` enables the `test-token` shortcut; set `production=true` in real deployments.

## Releasing

Consumers pin a **git tag** (e.g. `v0.1.5` in the README examples). Releasing is tagging a commit — there is no build artifact. CI (`.github/workflows/main.yml`) runs in a GOPATH layout and enforces `git diff --exit-code go.mod` after `go mod tidy`, so keep `go.mod` tidy or the build fails.

Full bump-and-deploy runbook (prod + beta): [doc/releasing.md](doc/releasing.md).
