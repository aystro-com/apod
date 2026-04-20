# apod

A single binary that turns any VPS into a hosting platform. Deploy sites, manage domains, handle SSL — all through Docker containers without the overhead of traditional panels.

## Why apod?

Hosting panels are bloated. PaaS platforms are expensive. Kubernetes is overkill for most workloads. apod sits in the sweet spot: one binary, zero dependencies beyond Docker, full isolation per site.

- **One binary** — drop it on a server and go
- **Docker-native** — every site runs in its own container
- **Automatic SSL** — Let's Encrypt via Traefik, zero config
- **Driver system** — define stacks as YAML (Laravel, WordPress, static, or roll your own)
- **Git deploys** — push to deploy with rollback support
- **Backups** — scheduled backups to S3, R2, SFTP, or local
- **CLI + REST API** — script everything, automate anything

## Quick start

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/aystro-com/apod/master/install.sh | sh

# Create a site
apod create myapp --driver laravel --domain myapp.com

# Deploy from git
apod deploy myapp --repo git@github.com:you/app.git --branch main

# Check status
apod list
```

## Drivers

Drivers are YAML configs that define your application stack. Ships with:

| Driver | Stack |
|--------|-------|
| `static` | Nginx |
| `laravel` | PHP + Nginx + MySQL |
| `wordpress` | WordPress + Apache + MySQL |

Create your own by dropping a YAML file in the drivers directory.

## Features

**Site management** — create, destroy, start, stop, restart with per-site resource limits (CPU, RAM)

**Domains & SSL** — multiple domains per site, automatic certificate provisioning and renewal

**Git deployments** — deploy from any Git repo with before/after hooks and instant rollback

**Databases** — per-site databases with export/import

**Backups** — scheduled backups with retention policies, multiple storage backends

**Cron jobs** — per-site scheduled tasks

**Monitoring** — uptime checks, container logs, server stats

**Firewall & security** — IP rules, port management, SSH keys, FTP accounts

**Proxy rules** — custom routing, redirects, basic auth

**Webhooks** — trigger deployments and actions from external services

## Architecture

```
apod (single binary)
├── CLI ─── commands that talk to the daemon
├── API ─── REST endpoints for everything the CLI can do
└── Engine
    ├── Docker ─── container lifecycle
    ├── Traefik ─── routing + SSL termination
    ├── Drivers ─── pluggable app stacks (YAML)
    ├── Scheduler ─── backup jobs + cron
    └── SQLite ─── state + config
```

## Requirements

- Linux VPS (Ubuntu 22.04+ recommended)
- Docker
- Ports 80 and 443 available

## License

MIT
