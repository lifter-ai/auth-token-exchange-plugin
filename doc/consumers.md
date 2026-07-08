# Deployment & Consumers

How this plugin is wired into the Duos stack — who registers it, what `authURL` it calls, and what actually reaches downstream services. Companion to [`../CLAUDE.md`](../CLAUDE.md) (plugin internals) and [`../README.md`](../README.md).

Source of truth: `infra/kubernetes/helm-charts/duos/traefik-values.yaml`, `helm-charts-sec/duos/{values,manifest}.yaml`, `ter-core` (`.../user/web/routes/UserCurrentRoutes.kt`, `.../integrations/dialogue/DialogueClient.kt`), and the `dialogue-api` repo (`index.ts`). As of 2026-07-08, all consumers pin **`version: v0.1.9`**.

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

## The critical wiring: `authURL` targets `/id`, not `/users/current`

Every consumer sets:

```yaml
authURL: http://core…:8080/api/v1/users/current/id
production: false
```

**Not `/users/current`.** `infra/CLAUDE.md` says the plugin "resolves the current user via `/users/current`" — that is **doc drift**. The real probe is the `/id` sub-route (`traefik-values.yaml:56`, `values.yaml:620`, `manifest.yaml:298`).

That endpoint (`ter-core` `UserCurrentRoutes.kt:41`) returns **`UserIdDto(id: Int)`** — the whole body is:

```json
{"id": 42}
```

So the plugin (`custom_auth.go:125-153`) propagates **only the id**:

| Header | Value | How |
|--|--|--|
| `X-User-Id` | `42` | `fmt.Sprintf("%v", userInfo["id"])` — JSON number → `float64` → `"42"` (the "fix user id to number" commit) |
| `X-User-Info` | `base64({"id":42})` | `base64(json.Marshal(body))` — **just the id**, not the user object |
| `X-Request-Id` | UUID v7 | minted per request |
| `Authorization` | *(deleted)* | credential never reaches downstream |

**Consequence.** The plugin's own docs frame `X-User-Info` as "the full user JSON", but because the config calls `/id`, the full `UserDto` (email, `premiumStatus`, `profile`, `groups`, `careerData` — the `/users/current` payload) is **never forwarded**. A downstream service needing more than the id must call `core` itself.

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
                                                     set X-User-Id=N, X-User-Info=base64({"id":N})
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

## Internal services behind `core`

Not everything needing user context sits behind Traefik. `core` fans out to internal services server-to-server, forwarding only a subset of what it resolved — the plugin is **not** in that path.

| Service | Reached via | Receives | Wants `X-User-Info`? |
|--|--|--|--|
| `dialogue-api` (`interviews-dialogue-api`) | `core` only — `http://dialogue-api:3000`, never Traefik | `X-User-Id` (explicit) + `X-Request-Id` | **No — reads neither** |

`ter-core`'s `DialogueClient` sets `X-User-Id` from `core`'s own principal; `X-Request-Id` rides the shared traced `HttpClient` (`ApplicationHttpClient.kt` `install(CallId)`). `dialogue-api`'s `headersValidation` hard-requires exactly those two (400 otherwise) and never references `X-User-Info` (`dialogue-api/index.ts:52-68`).

## Net `X-User-Info` receiver set

Three services — no more. `X-User-Id`/`X-Request-Id` ride the same middleware, so their guarded-route set is identical; `dialogue-api` gets those two one hop deeper, but never `X-User-Info`.

```
Traefik auth-token-exchange ──X-User-Info──▶ game-state-api · voice-api-v2 · core (beta interview-WS only)
core ──X-User-Id (+ X-Request-Id)──────────▶ dialogue-api        (never X-User-Info)
```

## Gotchas

- **`production: false` in prod and beta.** The `test-token` bypass (200 OK, request not forwarded — `custom_auth.go` shortcut) is **live in production**. Set `production: true` for real deployments; the current value looks unintended.
- **The `/id` probe has side effects.** `UserCurrentRoutes.kt:46-49` calls `reconcilePremiumStatus(user)` and appends its own `X-User-Id` *response* header on every hit. So each auth check also reconciles premium status — and the plugin ignores that response header, reading the body instead. The auth probe is doing more than authenticating.
- **Downstream expects richer `x-user-info` than it gets.** The converse WS engine documents `x-user-info` as "Base64-encoded UserInfo JSON" — in practice it is `base64({"id":N})`. Anything reading fields beyond `id` from that header will find nothing.
- **Deployed version may lag repo HEAD.** Consumers pin `v0.1.9`; the plugin repo `main` carries later commits. Bumping the plugin means re-tagging *and* updating `version:` in all three consumer files.
