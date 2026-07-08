# Deployment & Consumers

How this plugin is wired into the Duos stack — who registers it, what `authURL` it calls, and what reaches downstream services. Companion to [`../CLAUDE.md`](../CLAUDE.md) (plugin internals) and [`../README.md`](../README.md).

Source of truth: `infra/kubernetes/helm-charts/duos/traefik-values.yaml`, `helm-charts-sec/duos/{values,manifest}.yaml`, `ter-core` (`.../user/web/routes/UserCurrentRoutes.kt`, `.../integrations/dialogue/DialogueClient.kt`), and the `dialogue-api` repo (`index.ts`). Deployments pin **`version: v0.1.9`**; the header set below is the current code on `main` (see the `X-User-Info` note under [Gotchas](#gotchas)).

## Consumers

| Repo | File | Deploy target | Traefik provider |
|--|--|--|--|
| **infra** | `kubernetes/helm-charts/duos/traefik-values.yaml` | **prod** — `duostest-AksCluster`, ns `duos-test` | `extraObjects` → KubernetesCRD Middleware |
| **helm-charts-sec** | `duos/values.yaml` | **beta** — umbrella chart | file provider (`/etc/traefik/dynamic.yml`) |
| **helm-charts-sec** | `duos/manifest.yaml` | rendered snapshot of the beta config | file provider |

All three register the plugin identically and expose it as a Middleware named **`auth-token-exchange`**:

```yaml
experimental:
  plugins:
    auth-token-exchange-plugin:
      moduleName: github.com/lifter-ai/auth-token-exchange-plugin
      version: v0.1.9
```

## What the plugin forwards

On a 200 from `authURL` the plugin rewrites the request headers, then forwards it:

| Header | Value | How |
|--|--|--|
| `X-User-Id` | `42` | `fmt.Sprintf("%v", userInfo["id"])` — JSON number → `float64` → `"42"` (the "fix user id to number" commit) |
| `X-Request-Id` | UUID v7 | minted per request |
| `Authorization` | *(deleted)* | credential never reaches downstream |

**Only the id crosses the boundary.** A downstream service needing more than the id (email, `premiumStatus`, `profile`, …) must call `core` itself.

## The `authURL` targets `/id`, not `/users/current`

Every consumer sets:

```yaml
authURL: http://core…:8080/api/v1/users/current/id
production: false
```

**Not `/users/current`.** `infra/CLAUDE.md` says the plugin "resolves the current user via `/users/current`" — that is **doc drift**. The real probe is the `/id` sub-route (`traefik-values.yaml:56`, `values.yaml:620`, `manifest.yaml:298`), which returns **`UserIdDto(id: Int)`** (`ter-core` `UserCurrentRoutes.kt:41`) — the whole body is `{"id": 42}`. The plugin reads `userInfo["id"]` → `X-User-Id`; nothing else in the body is used.

## Request flow (as deployed)

```
client ──Bearer token──▶ AGIC/nginx edge ──▶ traefik:8000
                                               │
                              middleware: auth-token-exchange
                                               │  GET core:8080/api/v1/users/current/id
                                               │    (Authorization forwarded; ≤3 tries, backoff)
                                               ├─ 401 ─────────────▶ 401 "Invalid token"
                                               ├─ unreachable ─────▶ 500
                                               └─ 200 {"id":N}
                                                     set X-User-Id=N, X-Request-Id
                                                     del Authorization
                                               │
                              middleware: strip-api-prefix  (/api/v1, /api/v2)
                                               │
                                               ▼
                            core · game-state-api · voice-api-v2  (no credential; read X-User-Id)
```

## Where the middleware is applied

`auth-token-exchange` runs **before** `strip-api-prefix` on:

| Route | Service | Consumers |
|--|--|--|
| `Path(/api/v1/events)` \|\| `Path(/api/v1/state)` | `game-state-api` | prod + beta |
| `PathPrefix(/api/v2/voice/ws)` | `voice-api-v2` | prod + beta (also `strip-service-prefix`) |
| `Path(/api/v2/interview)` (interview WS) | `core` | beta only (`manifest.yaml:377`) |
| `PathPrefix(/api/v1)` \|\| `PathPrefix(/api/v2)` catch-all | `core` | prod — **no auth middleware on the catch-all** (`traefik-values.yaml:177`) |

`core` is the auth/user service: the `users-api`→`core` migration described in `infra/CLAUDE.md` is **already done** in these configs (`authURL` points at `core:8080`; `ter-core` *is* `core`).

**How each live consumer uses the id** (verified — none needs anything the plugin dropped):

| Service | Reads | On missing header |
|--|--|--|
| `game-state-api` (Go) | `x-user-id`, `x-request-id` (`main.go:128`) | — |
| `voice-api-v2` (Go) | no user/identity headers (only `Origin` on upgrade) | — |
| `core` converse WS | `x-user-id` → `userService.findById(userId)` from DB | connect refused only if `x-user-id` absent |

## Internal services behind `core`

Not everything needing user context sits behind Traefik. `core` fans out server-to-server, forwarding a subset of what it resolved — the plugin is **not** in that path.

| Service | Reached via | Receives |
|--|--|--|
| `dialogue-api` (`interviews-dialogue-api`) | `core` only — `http://dialogue-api:3000`, never Traefik | `X-User-Id` (explicit) + `X-Request-Id` |

`ter-core`'s `DialogueClient` sets `X-User-Id` from `core`'s own principal; `X-Request-Id` rides the shared traced `HttpClient` (`ApplicationHttpClient.kt` `install(CallId)`). `dialogue-api`'s `headersValidation` hard-requires exactly those two (400 otherwise) — `dialogue-api/index.ts:52-68`.

## Header receiver set

Only three routes chain `auth-token-exchange`, so only three services receive plugin-set headers — `X-User-Id` and `X-Request-Id` ride the same middleware, so their guarded-route set is identical:

```
Traefik auth-token-exchange ──X-User-Id + X-Request-Id──▶ game-state-api · voice-api-v2 · core (beta interview-WS only)
core ──X-User-Id (+ X-Request-Id)────────────────────────▶ dialogue-api   (one hop deeper, not via the plugin)
```

## Gotchas

- **`production: false` in prod and beta.** The `test-token` bypass (200 OK, request not forwarded — `custom_auth.go` shortcut) is **live in production**. Set `production: true` for real deployments; the current value looks unintended.
- **The `/id` probe has side effects.** `UserCurrentRoutes.kt:46-49` calls `reconcilePremiumStatus(user)` and appends its own `X-User-Id` *response* header on every hit. So each auth check also reconciles premium status — and the plugin ignores that response header, reading the body instead. The auth probe is doing more than authenticating.
- **`X-User-Info` was removed (2026-07-08).** Earlier builds — including the deployed `v0.1.9` — also set `X-User-Info` = base64 of the auth-service response body. It was dropped because nothing live needs it: `game-state-api`/`voice-api-v2` never read it, and `core`'s converse WS resolves the user from `X-User-Id` via the DB (`WebSocketConnectionService.kt` — *"the auth plugin only forwards the id"*). The legacy services that *required* it (`converse-api`, `interview-planner-api`, `job-mantra-api`) are not deployed behind the plugin. Bumping deployments off `v0.1.9` to the new tag is therefore safe.
- **Version bump touches three files.** `traefik-values.yaml`, `values.yaml`, and the generated `manifest.yaml` all pin `version:` independently — bump the source files and re-render `manifest.yaml`. Traefik re-fetches the plugin at the pinned tag only on pod restart. Full runbook: [releasing.md](releasing.md).
