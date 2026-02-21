# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

arnor is a unified infrastructure management CLI/TUI for managing web projects hosted on Hetzner Cloud with DNS via Porkbun or Cloudflare. It wraps three component tools — `shadowfax` (Porkbun DNS), `gwaihir` (Cloudflare DNS), and `fornost` (Hetzner Cloud) — into a single interface.

## Build and Development Commands

```bash
go build -o arnor .          # Build binary
go run .                      # Run directly
go test ./...                 # Run all tests
go test ./internal/dns/...    # Run tests for a specific package
go test -run TestFunctionName ./internal/dns/  # Run a single test
go vet ./...                  # Static analysis
go mod tidy                   # Clean up dependencies
```

## Technology Stack

- **Language:** Go
- **CLI framework:** Cobra + Viper for config
- **TUI framework (v2.0.0):** BubbleTea + Lipgloss + Bubbles
- **Env loading:** godotenv from `~/.dotfiles/.env`

## Architecture

### Key Design Principles

- Component tools (`shadowfax`, `gwaihir`, `fornost`) are imported as Go packages, not shelled out to as binaries. Their `internal/` client packages are the integration point.
- CLI functions must return structured data, not print directly to stdout, so the TUI layer (v2.0.0) can consume them without refactoring.
- No business logic in TUI models — the TUI is a thin wrapper over the same functions used by CLI commands.
- Mirror `.env` loading and error handling patterns from `shadowfax` for consistency across the suite.

### DNS Provider Interface

The central architectural piece. A common interface that both `shadowfax` (Porkbun) and `gwaihir` (Cloudflare) implement:

```go
type DNSProvider interface {
    CreateRecord(domain, name, recordType, content, ttl string) (string, error)
    DeleteRecord(domain, id string) error
    ListRecords(domain string) ([]DNSRecord, error)
}
```

Provider is auto-detected by inspecting the domain's nameservers.

### Project Structure

- `cmd/` — Cobra command definitions (root, config, project, service, server, dns, ssh)
- `internal/config/` — Config file read/write (`~/.config/arnor/config.yaml`)
- `internal/dns/` — DNS provider interface + Porkbun/Cloudflare adapters
- `internal/hetzner/` — Hetzner Cloud client adapter (wraps fornost)
- `internal/caddy/` — Caddy reverse proxy config generation + remote installation with cloudflare DNS module
- `internal/project/` — Project creation orchestration
- `internal/service/` — Service deployment orchestration (Docker Compose services without CI/CD)
- `tui/` — BubbleTea models and views (v2.0.0)

### Server Init Flow

`arnor server init --host <IP>` bootstraps a new server:

1. SSH as root (or sudo user) to run peon bootstrap script
2. Save peon SSH key to disk and database
3. Install Caddy with `caddy-dns/cloudflare` module (custom binary from caddyserver.com)
4. Create caddy user, systemd unit, `/etc/caddy/conf.d/`, log directory
5. If Cloudflare API token exists in store, write systemd override for `CF_API_TOKEN`
6. Enable and start Caddy

`arnor server caddy-setup --host <IP>` re-runs just the Caddy setup (steps 3-6) on an already-initialized server.

CF token lookup: `("cloudflare", "caddy", "api_token")` with fallback to `("cloudflare", "default", "api_token")`.

### Project Create Orchestration

The core workflow (`arnor project create`) replaces the old `new-project.sh`:

1. Prompt for GitHub repo, server, environment, domain, port
2. Look up server IP from Hetzner API
3. Detect DNS provider from nameservers
4. Create DockerHub repo via API
5. SSH into VPS as `peon` — create deploy user, deploy path, docker group, SSH keypair
6. Write Caddy config, restart Caddy
7. Create DNS A record and www CNAME
8. Set GitHub Actions secrets (namespaced by environment: `DEV_*` / `PROD_*`)
9. Generate workflow files into `.github/workflows/` (one per environment)
10. Write project entry to config

### Service Deploy Flow

`arnor service deploy` provides a lighter alternative to `project create` for deploying Docker Compose services (e.g. Uptime Kuma) without GitHub integration:

1. Prompt for service name, server, domain, port, path to docker-compose.yml
2. Look up server IP from config or Hetzner API
3. Detect DNS provider from nameservers
4. Create `/opt/{service-name}` on the server, owned by peon
5. Upload the local docker-compose.yml
6. Run `docker compose up -d`
7. Generate and deploy Caddy reverse proxy config (with validation)
8. Create DNS A record and www CNAME
9. Save as a Project with empty `Repo` field

Services are stored as regular Projects with `Repo: ""`. They appear with `(service)` in `arnor project list` and `arnor project view`. `arnor project inspect` skips GitHub checks for services.

No deploy user is created — peon runs everything directly. The flow reuses a single SSH connection for steps 4-7. Re-running for the same service is idempotent.

### Environment Conventions

- **dev:** subdomain under `angmar.dev`, DNS on Porkbun, triggered by push to `dev` branch
- **prod:** root domain from client, DNS provider auto-detected, triggered by semver tag (`v*`) on `main`
- Deploy users: `{project}-dev-deploy` and `{project}-deploy`
- Port allocation is manual; convention is dev = prod port + 1

## Configuration

- Environment variables: `~/.dotfiles/.env`
- Config file: `~/.config/arnor/config.yaml` (generated by `arnor config init`, not hand-written)
- `peon.sh` is a prerequisite on target servers — verify `peon` user exists before running `project create`
- `arnor server init` handles both peon bootstrap and Caddy installation — run it before `project create`
