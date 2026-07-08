# Releasing & Deploying a New Version

Cutting a new plugin version and rolling it out to prod + beta. There is **no build artifact** — a release is a git tag; deploying is bumping a pinned `version:` and restarting Traefik, which re-fetches the plugin on startup. Companion to [consumers.md](consumers.md) (where the plugin is wired) and [`../CLAUDE.md`](../CLAUDE.md).

Source of truth: `.github/workflows/main.yml` (the CI gate) and the three files that pin the version — `infra/kubernetes/helm-charts/duos/traefik-values.yaml`, `helm-charts-sec/duos/values.yaml`, `helm-charts-sec/duos/manifest.yaml`.

As of 2026-07-08 the latest tag is `v0.1.9`; the worked example below bumps to `v0.1.10`.

## Release model — four things that shape every step

- **Traefik *interprets* the plugin (Yaegi), it does not compile it.** The real gate is `make yaegi_test` — code can pass `go test` yet fail under Yaegi.
- **A release is a pushed git tag.** Traefik fetches `github.com/lifter-ai/auth-token-exchange-plugin` at the pinned tag, so the tag must exist on GitHub before any deploy references it.
- **Traefik fetches the plugin only at startup.** A version bump takes effect only after the Traefik pods restart.
- **The version is pinned in three files** (below) — bump all three; one is generated.

> ⚠️ Cluster / cloud steps (`kubectl`, `helmfile`, `terraform`, `az`, `helm push`) require the appropriate context and access. `helmfile -e prod sync` and anything on `duostest-AksCluster` **is production.**

## 1. Gate the code (plugin repo)

```bash
go test -v ./...                             # fast loop
make lint                                    # golangci-lint (CI pins v1.60.2)
make yaegi_test                              # THE gate — `yaegi test -v .`
go mod tidy && git diff --exit-code go.mod   # CI enforces a tidy go.mod
```

If `yaegi` / `golangci-lint` aren't installed locally, open a PR and let CI (`main.yml`) run all of it. Don't tag until it's green.

## 2. Tag the release

```bash
git commit -am "<change>"
git push
git tag v0.1.10 && git push origin v0.1.10   # the tag MUST reach GitHub — Traefik pulls the module at it
```

## 3. Bump the pin — three files, all currently `v0.1.9`

| Env | File:line | Action |
|--|--|--|
| **prod** | `infra/…/duos/traefik-values.yaml:37` | edit `version: v0.1.9` → `v0.1.10` |
| **beta** | `helm-charts-sec/duos/values.yaml:602` | edit `version: v0.1.9` → `v0.1.10` |
| **beta** | `helm-charts-sec/duos/manifest.yaml:418` | **do not hand-edit** — regenerate (step 4, beta) |

## 4. Deploy + restart Traefik

**Prod** (`infra/kubernetes`, context `duostest-AksCluster` = PROD):

```bash
kubectl config use-context duostest-AksCluster    # PROD
helmfile -e prod sync
kubectl rollout restart deploy/traefik -n duos-test   # force plugin re-fetch
```

**Beta** — `manifest.yaml` is generated and the umbrella chart is published to ACR, so after editing `values.yaml`:

```bash
cd helm-charts-sec
helm template duos ./duos -n duos --debug > ./duos/manifest.yaml   # regenerate snapshot (Makefile:90)
# bump duos/Chart.yaml: version 0.2.9-beta1 → 0.2.10-beta1
az acr login --name duostestacr
helm package duos --version 0.2.10-beta1
helm push duos-0.2.10-beta1.tgz oci://duostestacr.azurecr.io/charts

# then in infra/terraform/azure/environment/beta — duos.tf:13 chart_version → "0.2.10-beta1"
terraform plan && terraform apply
kubectl rollout restart deploy/traefik -n duos
```

## 5. Verify

- **Traefik loaded it:** `kubectl logs deploy/traefik -n <ns> | grep -i plugin` → version `v0.1.10`, no load error.
- **Auth still works:** hit a guarded route with a real bearer token → `200`, user resolves. Best targets: beta interview-WS (`/api/v2/interview`) or game-state `/api/v1/state`.
- **Behaviour change landed (optional):** confirm downstream logs reflect the change (e.g. for the X-User-Info removal, `core`/`game-state-api` no longer see that header).

## Notes

- **Consumers need no code changes** for the X-User-Info removal — `game-state-api` reads `x-user-id`, `voice-api-v2` reads no identity header, `core` resolves the user from `x-user-id` via the DB. See [consumers.md](consumers.md).
- **`production: false` is live in prod and beta** — the `test-token` bypass is enabled. A version bump is a natural moment to flip prod to `production: true`, but that disables the `test-token` shortcut integration tests may use — treat it as a separate decision.
