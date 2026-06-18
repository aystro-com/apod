# Testing apod

apod has three layers of tests. The first runs anywhere; the others need a Docker
daemon.

## 1. Unit & handler tests (no Docker)

```bash
go test ./...
```

Covers the engine's pure logic (process roles/replicas, validation, credential
parsers, …), the DB layer (SQLite), and the HTTP handlers. These run in CI and
must stay green.

## 2. Gated real-Docker integration tests

Some behaviour can only be verified against a live daemon (container creation,
scaling, replica cloning). Those tests are behind the `dockerintegration` build
tag so they never run in the normal suite:

```bash
go test -tags dockerintegration ./internal/engine/ -run Integration -v
```

They self-skip if no daemon is reachable. They use a tiny `alpine` image, so they
are fast and need no registry pulls beyond that.

## 3. Full lifecycle E2E (manual, real driver)

The integration tests stub the heavy app stack. To exercise a **real driver**
end to end — git deploy, `composer install`, web + worker + scheduler processes,
backup, and clone — drive the actual `apod` binary against a real daemon. This is
how the worker/clone features were validated; it catches a class of bugs that
mocked Docker hides (DB credential/datadir handling, init ordering, dump replay).

### Recipe (Linux host with Docker)

```bash
# 0. Build
go build -o /tmp/apod ./cmd/apod

# 1. Daemon (unix socket = local admin, no API key needed)
/tmp/apod server --db /tmp/apod.db --data-dir /tmp/apoddata --driver-dir ./drivers &

# 2. A Laravel app to deploy. apod refuses file:// repos, so serve it over git://
#    (composer install runs *inside* the container during deploy; commit vendor/
#    to keep the deploy hermetic, or ensure the container has registry access).
git daemon --reuseaddr --base-path=/tmp/gitserve --export-all --listen=127.0.0.1 &

# 3. Create + deploy
/tmp/apod create app.example.com --driver laravel \
    --repo git://127.0.0.1/app.git --branch main --deploy

# 4. Processes: scale the worker, restart, list
/tmp/apod process list  app.example.com
/tmp/apod process scale app.example.com queue 3
/tmp/apod process restart app.example.com scheduler

# 5. Backup, then clone into a brand-new site
/tmp/apod backup create app.example.com
/tmp/apod backup new-site app.example.com <backup-id> staging.example.com
```

Assert against the running containers, e.g.:

```bash
docker ps --filter label=apod.site=app.example.com \
  --format '{{.Names}} {{.Label "apod.service"}}/{{.Label "apod.role"}}#{{.Label "apod.replica"}}'
```

### What to check on a clone (`backup new-site`)

A clone must be *independent* of its source:

- the cloned app talks to **its own** DB container (`apod-<newdomain>-db`), not the
  source's — frameworks that cache resolved config (Laravel's
  `bootstrap/cache/config.php`) will pin the source host unless the cache is
  cleared; apod re-runs the driver's setup steps on import to handle this;
- the cloned DB keeps the **source's credentials** (apod records the live DB
  password in the backup metadata and reuses it) so the freshly-initialised DB,
  the restored `.env`, and the app all stay consistent;
- DB **data** comes from the logical dump (`mysqldump`/`pg_dump`), never from a
  raw copy of the live data directory — a hot file copy is not crash-consistent
  (Postgres refuses to start from one). Verify a non-MySQL stack (e.g. Postgres)
  clones too;
- the source site is untouched and keeps serving.

### Sandbox / CI-runner gotchas

These bit us while validating in a restricted environment; they are environmental,
not apod bugs:

- **Egress TLS interception.** If the host routes outbound traffic through a proxy
  with a custom CA, in-container `composer`/`npm`/`git-https` fail TLS. Mount the
  host CA bundle into the container and point the toolchain at it, e.g.
  `-v /etc/ssl/certs/ca-certificates.crt:/etc/ssl/certs/ca-certificates.crt:ro -e SSL_CERT_FILE=...`.
- **No container IPv6.** Some images (e.g. `webdevops/php-nginx`) ship nginx with
  `listen [::]:80`; if the daemon has IPv6 disabled, nginx crash-loops. Real hosts
  are unaffected; for a local smoke test you can serve via `php -S` to bypass nginx.

### Verifying site isolation

Sites must not reach each other. Each site's containers join only their
`apod-site-<domain>` network (never Docker's default bridge), so:

```bash
# From site A, neither name nor raw IP of site B's container should be reachable.
docker exec apod-a.example.com-app sh -c 'getent hosts apod-b.example.com-db || echo isolated-dns'
docker exec apod-a.example.com-app sh -c 'ping -c1 -W2 <site-B-ip> || echo isolated-l3'
```

The gated `TestIntegrationSiteIsolation` asserts this automatically.
