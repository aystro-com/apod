# apod

A single binary that turns any VPS into a hosting platform. Deploy sites, manage domains, handle SSL — all through Docker containers without the overhead of traditional panels.

## Why apod?

Hosting panels are bloated. PaaS platforms are expensive. Kubernetes is overkill for most workloads. apod sits in the sweet spot: one binary, zero dependencies beyond Docker, full isolation per site.

- **One binary** — drop it on a server and go
- **Docker-native** — every site runs in its own isolated container stack
- **Automatic SSL** — Let's Encrypt via Traefik, zero config
- **Driver system** — define stacks as YAML (PHP, Laravel, WordPress, Node.js, Odoo, or roll your own)
- **Git deploys** — push to deploy with rollback support and automatic pre-deploy backup
- **Backups** — databases (gzip-compressed) + site files + volume data, scheduled to S3/R2/SFTP/local
- **CLI + REST API** — script everything, automate anything
- **Multi-user** — Linux-level isolation with API key auth and ownership enforcement
- **Resource limits** — CPU, RAM, disk quotas, PID limits — all kernel-enforced
- **Network isolation** — each site gets its own Docker network, can't reach other sites
- **Billing integration** — WHMCS and Paymenter modules for automated provisioning
- **Migration** — export/import sites between servers with a single command
- **SaaS-ify anything** — turn any Docker app into a managed service in minutes
- **Web terminal** — secure token-based container shell access via billing panel

## Quick Start

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/aystro-com/apod/master/install.sh | sh

# Initialize (sets up systemd, SSL email, drivers)
apod init

# Create a PHP site with resource limits
apod create mysite.com --driver php --ram 512M --cpu 1 --storage 5G

# Deploy a Laravel app from git in one command
apod create myapp.com --driver laravel --repo https://github.com/you/app.git --branch main

# Deploy an Odoo ERP instance
apod create erp.mycompany.com --driver odoo --ram 2G --cpu 2 --storage 20G

# Shell into a site's container
apod access mysite.com

# Check status and resource usage
apod list
apod status mysite.com
apod top

# Create a user for multi-tenant hosting
apod user create client1 --role user
# → Returns an API key for remote management

# Update apod + drivers (auto-restarts)
apod update
```

## Table of Contents

- [Installation](#installation)
- [Configuration](#configuration)
- [Drivers](#drivers)
- [CLI Reference](#cli-reference)
- [REST API Reference](#rest-api-reference)
- [Billing Integrations](#billing-integrations)
- [Security Model](#security-model)
- [Architecture](#architecture)
- [Contributing](#contributing)

---

## Installation

### Requirements

- Linux server (Ubuntu 22.04+ recommended)
- Docker Engine 24.0+
- UFW firewall (recommended)
- Go 1.22+ (for building from source)
- Root access
- Ports 80 and 443 available
- `quota` package (for disk quota enforcement)

### Install Dependencies

```bash
# Install Docker
curl -fsSL https://get.docker.com | sh
systemctl enable docker && systemctl start docker

# Install UFW (firewall)
apt install -y ufw
ufw allow 22/tcp    # SSH
ufw allow 80/tcp    # HTTP
ufw allow 443/tcp   # HTTPS
ufw allow 8443/tcp  # apod API (if using remote access)
ufw --force enable

# Install quota tools (for disk limits)
apt install -y quota
```

### Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/aystro-com/apod/master/install.sh | sh
mkdir -p /etc/apod/drivers
apod update drivers
```

### From Source

```bash
git clone https://github.com/aystro-com/apod.git
cd apod
CGO_ENABLED=1 go build -o /usr/local/bin/apod ./cmd/apod/
mkdir -p /etc/apod/drivers
cp drivers/*.yaml /etc/apod/drivers/
```

### SystemD Service

Create `/etc/systemd/system/apod.service`:

```ini
[Unit]
Description=apod server orchestrator
After=docker.service
Requires=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/apod server --acme-email you@example.com
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable apod
systemctl start apod
```

### Updating

```bash
apod update              # Update binary + drivers + auto-restart daemon
apod update drivers      # Update built-in drivers only
apod version             # Check current version
```

---

## Configuration

### Daemon Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--acme-email` | | Email for Let's Encrypt certificates (required for SSL) |
| `--listen` | Unix socket | TCP address for remote API access (e.g., `0.0.0.0:8443`) |
| `--db` | `/etc/apod/apod.db` | SQLite database path |
| `--data-dir` | `/var/lib/apod` | Site data directory |
| `--driver-dir` | `/etc/apod/drivers` | Driver YAML directory |

### Data Layout

```
/etc/apod/
  apod.db                 # All state (sites, configs, schedules, logs)
  drivers/
    static.yaml
    wordpress.yaml
    laravel.yaml

/var/lib/apod/                        # Admin-owned sites
  sites/
    example.com/
      files/                          # Site code (mounted into container)
      data/
        mysql/                        # Database files
  backups/                            # Admin site backups
    example.com/
      example.com_20260420_120000.zip

/home/<user>/                         # User-owned sites
  sites/
    mysite.com/
      files/
      data/
  backups/                            # User backups (counts against disk quota)
    mysite.com/
      mysite.com_20260420_120000.zip
```

### Remote Access

```bash
# Start daemon with TCP listener
apod server --listen 0.0.0.0:8443 --acme-email you@example.com

# Connect from another machine
apod --remote https://your-server:8443 --key <api-key> list
```

---

## Drivers

Drivers are YAML files that define application stacks. Each driver specifies Docker images, volumes, ports, environment, deploy hooks, health checks, backup targets, and setup steps.

### Built-in Drivers

| Driver | Stack | Image |
|--------|-------|-------|
| `static` | Nginx | `nginx:alpine` |
| `php` | PHP + Nginx + MySQL (blank, no git) | `webdevops/php-nginx-dev:8.4` + `mysql:8.0` |
| `wordpress` | WordPress + Apache + MySQL | `wordpress:php8.3-apache` + `mysql:8.0` |
| `laravel` | PHP 8.4 + Nginx + MySQL | `webdevops/php-nginx-dev:8.4` + `mysql:8.0` |
| `node` | Node.js + PostgreSQL | `node:22-alpine` + `postgres:16-alpine` |
| `odoo` | Odoo ERP + PostgreSQL | `odoo:17.0` + `postgres:16-alpine` |
| `unifi` | UniFi Network Controller + MongoDB | `jacobalberty/unifi:latest` + `mongo:4.4` |
| `paymenter` | Paymenter billing + MySQL + Redis | `webdevops/php-nginx-dev:8.3` + `mysql:8.0` + `redis:7` |
| `whmcs` | WHMCS + MySQL + ionCube | `php:8.2-apache` + `mysql:8.0` |

**SaaS-ify any app:** Write a 20-40 line YAML driver for any Docker app, connect a billing panel, and sell managed instances. We went from zero to selling managed Odoo in under 30 minutes.

### Writing a Custom Driver

Create a YAML file in `/etc/apod/drivers/`. Example for a Node.js app:

```yaml
name: nodejs
version: "1.0"
description: Node.js application with MongoDB

parameters:
  node_version:
    type: string
    default: "22"
    options: ["18", "20", "22"]

services:
  app:
    image: "node:${node_version}-alpine"
    volumes:
      - "${site_root}:/app"
    ports:
      - "3000"
    environment:
      NODE_ENV: "production"
      MONGO_URL: "mongodb://apod-${site_domain}-db:27017/${site_db_name}"
    command: "cd /app && node server.js"

  db:
    image: "mongo:7"
    volumes:
      - "${data_root}/mongo:/data/db"

deploy:
  before_deploy:
    - "cd /app && npm ci --production"
  after_deploy:
    - "cd /app && npx prisma migrate deploy"

healthcheck:
  url: "http://localhost:3000/health"
  interval: 10s
  timeout: 5s
  retries: 3

backup:
  paths:
    - "${site_root}"
  databases:
    - type: mongo
      service: db

cron:
  - schedule: "0 * * * *"
    command: "cd /app && node scripts/cleanup.js"
    service: app

setup:
  - name: "Install dependencies"
    command: "cd /app && npm ci --production"
    service: app
```

### Driver Variables

| Variable | Description |
|----------|-------------|
| `${site_root}` | Site files directory (`/var/lib/apod/sites/<domain>/files`) |
| `${data_root}` | Persistent data directory (`/var/lib/apod/sites/<domain>/data`) |
| `${site_domain}` | Site primary domain |
| `${site_db_name}` | Auto-generated database name |
| `${site_db_user}` | Auto-generated database user |
| `${site_db_pass}` | Auto-generated database password |

Driver parameters (defined in `parameters:`) are also available as variables. For example, `${node_version}` resolves to the parameter's default or the value passed at creation.

### Driver Sections

| Section | Required | Description |
|---------|----------|-------------|
| `services` | Yes | Docker containers to create (image, volumes, ports, env, command, backend_scheme) |
| `parameters` | No | User-configurable values with defaults and options |
| `deploy` | No | `before_deploy` and `after_deploy` hook commands for git deploys |
| `healthcheck` | No | HTTP endpoint to verify site health |
| `backup` | No | Paths and databases to include in backups |
| `cron` | No | Default cron jobs created with the site |
| `setup` | No | Commands to run after initial site creation (supports `user: root`) |

**Service options:**
- `backend_scheme: "https"` — tells Traefik the backend uses HTTPS (e.g., UniFi controller)

**Setup step options:**
- `user: root` — run the setup command as root inside the container (useful for fixing permissions)

---

## CLI Reference

### Sites

```bash
apod init                                # First-run setup wizard
apod create <domain> --driver <name> [--ram 256M] [--cpu 1] [--storage 5G] [--repo <url>] [--branch main] [--deploy]
apod destroy <domain> [--purge]          # --purge removes all data
apod start <domain>
apod stop <domain>
apod restart <domain>
apod list                                # List all sites
apod status <domain>                     # Detailed site info + resource usage
apod access <domain> [--shell bash]      # Interactive shell into container
apod clone <source> <target>             # Full site copy
apod export <domain> [-o /path/]         # Export site to zip for migration
apod import <file.zip> [--domain new]    # Import site from export zip
```

### Migration (VPS to VPS)

Move a site between servers with a single export/import:

```bash
# On source server
apod export mysite.com -o /tmp/
# → /tmp/mysite.com_export_20260421_120000.zip

# Transfer to target server
scp /tmp/mysite.com_export_*.zip root@new-server:/tmp/

# On target server
apod import /tmp/mysite.com_export_*.zip

# Or import with a different domain
apod import /tmp/mysite.com_export_*.zip --domain newdomain.com

# Or assign to a user
apod import /tmp/mysite.com_export_*.zip --owner client1
```

The export includes everything: site files, volume data, gzip-compressed database dumps, env vars, domain aliases, and resource config metadata.

### Domains

All domains get automatic SSL via Let's Encrypt.

```bash
apod domain add <site-domain> <new-domain>
apod domain remove <site-domain> <alias>
apod domain list <site-domain>
```

### Resource Limits

All limits are enforced at the kernel/Docker level — no bypass possible. Tested against crypto miners, RAM bombs, fork bombs, and disk bombs.

```bash
apod create mysite.com --driver php --ram 512M --cpu 2 --storage 10G
apod config set mysite.com --set-key ram --set-value 1G
apod config set mysite.com --set-key storage --set-value 20G
```

| Resource | Flag | Enforcement | Effect |
|----------|------|-------------|--------|
| RAM | `--ram 256M` | Docker memory limit | OOM kill inside container only, auto-restart |
| CPU | `--cpu 1` | Docker CPU limit | Hard cap per core, other sites unaffected |
| Disk | `--storage 5G` | Linux `setquota` on user UID | `Disk quota exceeded` error on write |
| Processes | Default 512 | Docker PidsLimit | Fork bombs hit limit and stop |

**Process limit:** The default PID limit is 512 per container (sufficient for PHP-FPM, MySQL, Node.js, etc.). If a site needs more (e.g., a heavy Java app), increase it in the driver or per-site config.

**Disk quota setup** (one-time, required for `--storage` to work):

```bash
apt install quota
mount -o remount,usrquota /
quotacheck -cum /
quotaon /
```

Add `usrquota` to `/etc/fstab` for persistence across reboots.

Disk quotas apply per user — the total storage for all of a user's sites is summed and enforced as one quota on their Linux UID. Admin-owned sites (no `--owner`) have no disk quota.

**Network isolation:** Each site gets its own Docker network. Sites cannot resolve, connect to, or port-scan other sites' containers or databases. Only Traefik connects to all site networks for routing.

### Configuration

```bash
apod config get <domain>
apod config set <domain> --set-key <key> --set-value <value>
```

Keys: `ram`, `cpu`, `storage`, `repo`, `branch`

### Environment Variables

```bash
apod env set <domain> KEY=VALUE [KEY2=VALUE2 ...]
apod env list <domain>
apod env unset <domain> KEY [KEY2 ...]
```

### Git Deploy

```bash
apod deploy <domain> [--branch <branch>]    # Pull, install deps, run hooks
apod rollback <domain>                       # Revert to previous deploy
apod deploy list <domain>                    # Deployment history
```

### Webhooks

```bash
apod webhook create <domain>     # Returns token + URL
apod webhook list <domain>
apod webhook delete <domain>
```

External push-to-deploy URL: `POST https://<server>/webhook/<token>`

Use this in GitHub/GitLab webhook settings — any push triggers a deploy.

### Backups

Each backup includes:
- **Database dumps** (gzip-compressed) — MySQL, PostgreSQL, MongoDB
- **Site files** — application code from `${site_root}`
- **Volume data** — persistent data from `${data_root}` (auto-included if not in driver paths)
- **Metadata** — domain, driver, env vars, resource config

Backups are verified after creation (empty backups are rejected). User-owned site backups are stored in `/home/<user>/backups/` and count against the user's disk quota. Admin site backups go to `/var/lib/apod/backups/`.

```bash
apod backup create <domain> [--storage <name>]
apod backup list <domain>
apod backup restore <domain> <backup-id>
apod backup delete <domain> <backup-id>
```

**Auto backup before deploy:** Every `apod deploy` automatically creates a backup first, so you can always roll back safely.

**Scheduled backups:**

```bash
apod backup schedule add <domain> --every <interval> --keep <count> [--storage <name>]
apod backup schedule list <domain>
apod backup schedule remove <domain> <schedule-id>
```

Intervals: `hourly`, `daily`, `weekly`, `monthly` (or `1h`, `6h`, `12h`, `24h`, `7d`, `30d`)

### Backup Storage

Local storage is always available as the default. Add remote storage:

```bash
# Amazon S3 (or any S3-compatible: MinIO, DigitalOcean Spaces, Backblaze B2)
apod storage add my-s3 --driver s3 \
  --bucket backups --region us-east-1 \
  --access-key AKIA... --secret-key ...

# Cloudflare R2
apod storage add my-r2 --driver r2 \
  --bucket backups --account-id abc123 \
  --access-key ... --secret-key ...

# SFTP
apod storage add my-sftp --driver sftp \
  --host backup.example.com --user backups \
  --password ... --path /backups

apod storage list
apod storage remove <name>
```

### Cron Jobs

Jobs execute inside the site's container.

```bash
apod cron add <domain> --schedule "*/5 * * * *" --command "php artisan schedule:run"
apod cron list <domain>
apod cron remove <domain> <cron-id>
```

### Monitoring

```bash
apod top                         # Live CPU/RAM for all sites
apod server-stats                # Server totals (CPU, RAM, disk, site count)
apod disk-usage                  # Disk usage per site
apod tail <domain>               # Container stdout/stderr (last 100 lines)
apod tail <domain> -f            # Follow log output in real time
apod tail <domain> -n 50         # Show last 50 lines
```

### Uptime Monitoring

```bash
apod uptime enable <domain> --url https://example.com [--interval 60] [--alert-webhook <url>]
apod uptime disable <domain>
apod uptime status <domain>      # Uptime %, avg response time, total checks
apod uptime logs <domain>        # Recent check history
```

Alert webhook payload (sent on UP/DOWN transitions):

```json
{
  "domain": "example.com",
  "status": "down",
  "status_code": 500,
  "timestamp": "2026-04-20T15:00:00Z"
}
```

### Database

```bash
apod db export <domain> > dump.sql
apod db import <domain> dump.sql
```

### Security

**Proxy rules:**

```bash
apod proxy add <domain> --type redirect --from /old --to /new
apod proxy add <domain> --type header --name X-Frame-Options --value DENY
apod proxy add <domain> --type basic-auth --user admin --password secret
apod proxy list <domain>
apod proxy remove <domain> <rule-id>
```

**IP blocking:**

```bash
apod ip block <domain> <ip>
apod ip unblock <domain> <ip>
apod ip list <domain>
```

**Firewall (UFW):**

```bash
apod firewall status
apod firewall enable
apod firewall allow <port>
apod firewall deny <port>
```

**SSH keys:**

```bash
apod ssh-key add <name> "<public-key>"
apod ssh-key list
apod ssh-key remove <name>
```

**FTP/SFTP accounts:**

```bash
apod ftp add <domain> --user <username> --password <password>
apod ftp list <domain>
apod ftp remove <domain> <username>
```

### User Management

Multi-user support with Linux-level isolation. Each user gets their own Linux user, chrooted SFTP access, and sites under `/home/<user>/sites/`.

```bash
apod user create <name> [--role user|admin]  # Creates Linux user + API key
apod user list                               # List all users
apod user delete <name>                      # Remove user (must have no sites)
apod user reset-key <name>                   # Generate new API key
apod transfer <domain> <new-owner>           # Transfer site to another user
apod transfer <domain> ""                    # Unassign site (admin-owned)
```

**How it works:**
- Each user gets a real Linux user (UID 5000+) with a home directory
- Sites created by a user live under `/home/<user>/sites/<domain>/`
- SFTP access is chrooted — users can only see their own sites
- API keys are SHA-256 hashed (shown only once on create/reset)
- Users can only manage their own sites via the API
- Admins see and control everything
- Unix socket access (local) is always admin

**Remote access as a user:**
```bash
apod --remote https://server:8443 --key apod_<key> list
apod --remote https://server:8443 --key apod_<key> create mysite.com --driver php
```

### Activity Log

```bash
apod logs                    # All operations across all sites
apod logs <domain>           # Operations for a specific site
```

### System

```bash
apod version                 # Show version + DB schema version
apod update                  # Self-update binary + drivers + auto-restart daemon
apod update drivers          # Pull latest driver YAMLs only
apod driver list             # Show installed drivers
apod init                    # First-run setup (Docker check, SSL email, systemd)
```

### Server Daemon

```bash
apod server --acme-email you@example.com                    # Listen on Unix socket only
apod server --acme-email you@example.com --listen 0.0.0.0:8443  # Socket + TCP (for remote/billing API)
```

When `--listen` is set, both the Unix socket (admin, local) and TCP (authenticated, remote) listeners run simultaneously.

---

## REST API Reference

Every CLI command maps to an API endpoint. The API listens on a Unix socket (`/var/run/apod.sock`) by default, or on a TCP port with `--listen`.

### Authentication

```
Authorization: Bearer <api-key>
```

### Response Format

All responses follow this structure:

```json
{
  "ok": true,
  "data": { ... }
}
```

Error responses:

```json
{
  "ok": false,
  "error": "description of what went wrong"
}
```

### Sites

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/sites` | Create site | `{"domain", "driver", "ram", "cpu", "storage", "repo", "branch"}` |
| `GET` | `/api/v1/sites` | List all sites | |
| `GET` | `/api/v1/sites/{domain}` | Get site details | |
| `POST` | `/api/v1/sites/{domain}/start` | Start site | |
| `POST` | `/api/v1/sites/{domain}/stop` | Stop site | |
| `POST` | `/api/v1/sites/{domain}/restart` | Restart site | |
| `DELETE` | `/api/v1/sites/{domain}` | Destroy site | `?purge=true` to remove data |
| `POST` | `/api/v1/sites/{domain}/clone` | Clone site | `{"target": "new.domain.com"}` |
| `POST` | `/api/v1/sites/{domain}/export` | Export site to zip | `{"output_dir": "/tmp"}` |
| `POST` | `/api/v1/import` | Import site from zip | `{"path": "/tmp/export.zip", "domain": "", "owner": ""}` |

### Domains

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `GET` | `/api/v1/sites/{domain}/domains` | List domains | |
| `POST` | `/api/v1/sites/{domain}/domains` | Add domain | `{"domain": "alias.com"}` |
| `DELETE` | `/api/v1/sites/{domain}/domains/{alias}` | Remove domain | |

### Configuration

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `GET` | `/api/v1/sites/{domain}/config` | Get all config | |
| `POST` | `/api/v1/sites/{domain}/config` | Set config value | `{"key": "ram", "value": "1G"}` |

### Environment Variables

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `GET` | `/api/v1/sites/{domain}/env` | List env vars | |
| `POST` | `/api/v1/sites/{domain}/env` | Set env var | `{"key": "DB_HOST", "value": "localhost"}` |
| `DELETE` | `/api/v1/sites/{domain}/env/{key}` | Remove env var | |

### Deploy

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/sites/{domain}/deploy` | Trigger deploy | `{"branch": "main"}` |
| `POST` | `/api/v1/sites/{domain}/rollback` | Rollback | |
| `GET` | `/api/v1/sites/{domain}/deployments` | List deployments | |

### Webhooks

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/sites/{domain}/webhook` | Create webhook | |
| `GET` | `/api/v1/sites/{domain}/webhook` | List webhooks | |
| `DELETE` | `/api/v1/sites/{domain}/webhook` | Delete webhook | |
| `POST` | `/webhook/{token}` | Incoming webhook (triggers deploy) | Any (e.g., GitHub push payload) |

### Backups

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/sites/{domain}/backups` | Create backup | `{"storage": "my-s3"}` |
| `GET` | `/api/v1/sites/{domain}/backups` | List backups | |
| `POST` | `/api/v1/sites/{domain}/backups/restore` | Restore backup | `{"backup_id": 1}` |
| `DELETE` | `/api/v1/sites/{domain}/backups` | Delete backup | `{"backup_id": 1}` |

### Backup Schedules

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/sites/{domain}/backups/schedule` | Add schedule | `{"every": "24h", "keep": 7, "storage": ""}` |
| `GET` | `/api/v1/sites/{domain}/backups/schedule` | List schedules | |
| `DELETE` | `/api/v1/sites/{domain}/backups/schedule` | Remove schedule | `{"schedule_id": 1}` |

### Storage

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/storage` | Add storage config | `{"name", "driver", "config": {"bucket": "..."}}` |
| `GET` | `/api/v1/storage` | List storage configs | |
| `DELETE` | `/api/v1/storage/{name}` | Remove storage config | |

### Cron Jobs

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/sites/{domain}/cron` | Add cron job | `{"schedule": "* * * * *", "command": "...", "service": "app"}` |
| `GET` | `/api/v1/sites/{domain}/cron` | List cron jobs | |
| `DELETE` | `/api/v1/sites/{domain}/cron` | Remove cron job | `{"id": 1}` |

### Monitoring

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/sites/{domain}/monitor` | Site CPU/RAM stats |
| `GET` | `/api/v1/monitor` | All sites stats |
| `GET` | `/api/v1/server-stats` | Server totals (CPU, RAM, disk) |
| `GET` | `/api/v1/disk-usage` | Per-site disk usage |
| `GET` | `/api/v1/sites/{domain}/container-logs` | Container stdout/stderr |

### Uptime

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/sites/{domain}/uptime` | Enable monitoring | `{"url", "interval": 60, "alert_webhook": ""}` |
| `GET` | `/api/v1/sites/{domain}/uptime` | Get status + stats | |
| `DELETE` | `/api/v1/sites/{domain}/uptime` | Disable monitoring | |
| `GET` | `/api/v1/sites/{domain}/uptime/logs` | Check history | |

### Database

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `GET` | `/api/v1/sites/{domain}/db/export` | Export dump | |
| `POST` | `/api/v1/sites/{domain}/db/import` | Import dump | `{"dump": "SQL content..."}` |

### Security

**Proxy rules:**

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/sites/{domain}/proxy` | Add rule | `{"type": "redirect", "config": {"from": "/old", "to": "/new"}}` |
| `GET` | `/api/v1/sites/{domain}/proxy` | List rules | |
| `DELETE` | `/api/v1/sites/{domain}/proxy` | Remove rule | `{"id": 1}` |

**IP blocking:**

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/sites/{domain}/ip/block` | Block IP | `{"ip": "1.2.3.4"}` |
| `POST` | `/api/v1/sites/{domain}/ip/unblock` | Unblock IP | `{"ip": "1.2.3.4"}` |
| `GET` | `/api/v1/sites/{domain}/ip` | List rules | |

**FTP accounts:**

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/sites/{domain}/ftp` | Add account | `{"username", "password"}` |
| `GET` | `/api/v1/sites/{domain}/ftp` | List accounts | |
| `DELETE` | `/api/v1/sites/{domain}/ftp/{username}` | Remove account | |

**Firewall:**

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `GET` | `/api/v1/firewall` | Status + rules | |
| `POST` | `/api/v1/firewall/enable` | Enable UFW | |
| `POST` | `/api/v1/firewall/allow` | Allow port | `{"port": "3306"}` |
| `POST` | `/api/v1/firewall/deny` | Deny port | `{"port": "3306"}` |

**SSH keys:**

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/ssh-keys` | Add key | `{"name", "public_key"}` |
| `GET` | `/api/v1/ssh-keys` | List keys | |
| `DELETE` | `/api/v1/ssh-keys/{name}` | Remove key | |

### Users (admin only)

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/users` | Create user | `{"name", "role": "user"}` |
| `GET` | `/api/v1/users` | List users | |
| `DELETE` | `/api/v1/users/{name}` | Delete user | |
| `POST` | `/api/v1/users/{name}/reset-key` | Reset API key | |
| `POST` | `/api/v1/sites/{domain}/transfer` | Transfer site ownership | `{"owner": "newuser"}` |

### Terminal (secure container exec)

| Method | Endpoint | Description | Body |
|--------|----------|-------------|------|
| `POST` | `/api/v1/sites/{domain}/terminal` | Generate exec token (5min TTL) | |
| `POST` | `/api/v1/terminal/exec` | Execute command with token | `{"token": "term_...", "command": "ls"}` |

Token-based access — no API key needed for exec, the token IS the auth. Security features:
- Tokens expire after 5 minutes
- Single-domain scoped (can't access other sites)
- 100 command limit per token
- Dangerous commands blocked (mount, insmod, reboot, etc.)
- Output capped at 64KB
- Commands run inside the site's app container only — never the host

### Activity Log

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/sites/{domain}/logs` | Site activity log |
| `GET` | `/api/v1/logs` | Global activity log |

### System

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/version` | App version + DB schema version |
| `GET` | `/api/v1/update/check` | Check for updates |
| `POST` | `/api/v1/update` | Self-update binary |
| `POST` | `/api/v1/update/drivers` | Update driver YAMLs |
| `GET` | `/api/v1/drivers` | List installed drivers |

---

## Architecture

```
apod (single binary, ~15k lines of Go)
  CLI ──── commands that talk to the daemon via Unix socket or HTTP
  API ──── REST endpoints for everything the CLI can do
  Engine
    Docker ──── container lifecycle, image pulls, exec, per-site networks
    Traefik ──── reverse proxy, SSL termination, routing
    Drivers ──── pluggable app stacks defined as YAML
    Users ────── multi-user with Linux UID isolation
    Quotas ───── CPU, RAM, disk, PID limits
    Terminal ─── secure token-based container exec
    Scheduler ── backup schedules + cron jobs (robfig/cron)
    Uptime ───── background HTTP checker with alerts
    SQLite ───── all state in one file (versioned migrations)
  Billing
    WHMCS ────── provisioning module (PHP)
    Paymenter ── server extension (PHP)
```

### How Routing Works

1. `apod create` spins up containers with Traefik labels
2. Traefik auto-discovers containers via Docker socket
3. Traefik routes traffic based on `Host()` rules in labels
4. SSL certificates provisioned automatically via HTTP challenge
5. HTTP requests redirect to HTTPS

### How Deploys Work

1. `apod deploy` runs `git pull` in the site's files directory
2. Runs `before_deploy` hooks (e.g., `composer install`)
3. Restarts site containers
4. Runs `after_deploy` hooks (e.g., `php artisan migrate`)
5. Records deployment in activity log

### How Backups Work

1. Database dump via `docker exec` (mysqldump, pg_dump, mongodump)
2. Site files copied from volume
3. Metadata exported (env vars, config, domains)
4. Everything zipped and stored (local or remote)
5. Retention policy deletes old backups

### Project Structure

```
cmd/apod/              Entry point
internal/
  cli/                 Cobra commands (one file per command group)
  db/                  SQLite layer (one file per table)
  engine/              Business logic (one file per feature)
  models/              Data structures
  server/              REST API (chi router)
  storage/             Backup storage drivers (local, S3, R2, SFTP)
drivers/               Built-in driver YAML files
```

---

## Billing Integrations

apod ships with billing panel modules for automated provisioning. Customers purchase hosting plans, and sites are created/suspended/terminated automatically.

### WHMCS

Install: copy `extensions/whmcs/modules/servers/apod/` to your WHMCS `/modules/servers/` directory.

**Server setup**: Add a server with hostname, port (8443), and admin API key as password.

**Product ConfigOptions** (1-6):
1. Driver (php, laravel, wordpress, node, odoo, etc.)
2. RAM limit (256M, 512M, 1G)
3. CPU cores (1, 2, 4)
4. Storage quota (1G, 5G, 10G)
5. Shell Access (yes/no) — web terminal to container
6. Backups (yes/no) — customer can create/restore backups

**Features:**
- Auto-provision on payment
- Suspend/unsuspend/terminate
- Client area: site stats, resource usage, backup list, restart button
- Admin area: site details, driver info, quick actions
- Web terminal: secure token-based container exec (no host access)

### Paymenter

Install: copy `extensions/paymenter/Apod.php` to your Paymenter `/extensions/Servers/Apod/` directory.

**Server setup**: Configure with apod host URL and admin API key.

**Features:**
- Same provisioning lifecycle — create, suspend, unsuspend, terminate
- Product configuration: driver, RAM, CPU, storage per product
- Fetches available drivers from apod API dynamically

### SaaS-ify Any App

The billing integration makes it trivial to turn any Docker application into a managed service:

1. Write a YAML driver (20-40 lines) for your app
2. Add it to `/etc/apod/drivers/`
3. Create a product in WHMCS/Paymenter with pricing
4. Customers buy → isolated instance provisioned automatically with SSL

We tested this with Odoo ERP — from idea to selling managed instances in under 30 minutes. The same approach works for n8n, Metabase, Gitea, Nextcloud, or any custom application.

---

## Security Model

Every site is fully isolated. Tested against CPU miners, RAM bombs, fork bombs, disk bombs, network attacks, and container escape attempts.

**Resource isolation (kernel-enforced):**
- **CPU**: Docker `NanoCPUs` — hard cap per core. A crypto miner in one container can't affect others.
- **RAM**: Docker `Memory` — OOM killer scoped to the container only. Other sites unaffected.
- **Disk**: Linux `setquota` — writes fail with "Disk quota exceeded" at the limit.
- **Processes**: `PidsLimit` (default 512, configurable) — fork bombs hit the limit and stop.

**Network isolation:**
- Each site gets its own Docker network (`apod-site-<domain>`). Only that site's containers and Traefik are connected.
- Sites cannot resolve, ping, or connect to other sites' containers or databases.

**Container hardening:**
- All Linux capabilities dropped (`CapDrop: ALL`), only 6 minimal ones added (CHOWN, DAC_OVERRIDE, FOWNER, SETGID, SETUID, NET_BIND_SERVICE)
- `no-new-privileges` prevents privilege escalation
- No Docker socket access
- No host filesystem visibility
- Cannot mount filesystems, change sysctl, load kernel modules, or change hostname
- Container only sees its own processes (not host processes)

**Access control:**
- **API auth**: SHA-256 hashed API keys, role-based (admin vs user)
- **Ownership**: Users can only see/manage their own sites — enforced on every endpoint
- **Rate limiting**: 60 requests/minute per IP on TCP connections (Unix socket bypasses)
- **Web terminal**: Token-based (5min TTL, 100 command limit), word-boundary command filtering blocks dangerous operations and shell escapes (`$()`, backticks)
- **Multi-user**: Linux user isolation with SFTP chroot for admin/agency users
- **SSL**: Automatic Let's Encrypt via Traefik

**Input validation:**
- Domain names validated against strict regex (prevents container name injection)
- Firewall ports validated (prevents command injection via ufw)
- SSRF protection on uptime URLs and webhooks (blocks private IPs, loopback, metadata endpoints)
- Database import uses base64 encoding (prevents shell injection via SQL dump content)
- Error messages sanitized — 500 errors log details server-side, return generic message to client
- Backup downloads validated against path traversal, zip restore protected against zip-slip

---

## Contributing

```bash
# Clone and build
git clone https://github.com/aystro-com/apod.git
cd apod && go build ./...

# Run tests
go test ./...

# Project conventions
# - TDD: write tests first
# - One file per feature/table
# - CLI commands are thin wrappers around API calls
# - Engine methods do the real work
```

## License

MIT
