# arnor

A unified infrastructure management CLI for managing web projects hosted on Hetzner Cloud with DNS via Porkbun or Cloudflare. Wraps three component tools — `shadowfax` (Porkbun DNS), `gwaihir` (Cloudflare DNS), and `fornost` (Hetzner Cloud) — into a single interface.

## Setup

### Prerequisites

- Go 1.25+
- [gh CLI](https://cli.github.com/) (for `project create`)
- `peon` user provisioned on target servers

### Install

```bash
go build -o arnor .
```

### Initialize

All configuration and credentials are stored in a SQLite database at `~/.config/arnor/arnor.db`. On first run, set up your Hetzner project:

```bash
arnor config init
```

This prompts for a Hetzner API token, validates it, and auto-discovers your servers.

### Add credentials

```bash
arnor config add porkbun default api_key pk1_xxx
arnor config add porkbun default secret_key sk1_xxx
arnor config add cloudflare default api_token cf_xxx
arnor config add dockerhub default username myuser
arnor config add dockerhub default password mypass
arnor config add dockerhub default token dckr_pat_xxx  # optional PAT for CI
```

## Usage

### Config

```bash
arnor config init              # Interactive setup: add Hetzner token, discover servers
arnor config view              # Print current config from DB
arnor config add <svc> <name> <key> <value>  # Set a credential
```

### Servers

```bash
arnor server list              # List all servers across Hetzner projects
arnor server view my-vps       # Show details for a specific server
arnor server init --host 1.2.3.4  # Bootstrap peon deploy user on a VPS
```

### DNS

DNS provider is auto-detected from the domain's nameservers.

```bash
arnor dns list --domain example.com
arnor dns create --domain example.com --type A --content 1.2.3.4
arnor dns create --domain example.com --name www --type CNAME --content example.com
arnor dns delete --domain example.com --id 12345
```

### Projects

```bash
arnor project list             # List all configured projects
arnor project view myclient    # Show project details with environments
arnor project create           # Interactive wizard for full project setup
arnor project inspect myclient # Show GitHub secrets and workflow runs
```

### Deploy

```bash
arnor deploy myclient --env dev   # Trigger GitHub Actions deploy
arnor deploy myclient --env prod
```

### TUI

```bash
arnor tui
```

Interactive terminal UI with screens for server init, project creation, deploy, project inspect, and Docker container viewing. On first run with an empty database, a setup wizard appears to configure your first Hetzner project.

### Project Create Workflow

`arnor project create` is the primary function — it replaces manual server setup with an interactive wizard that:

1. Looks up the server IP from Hetzner
2. Detects the DNS provider from nameservers
3. Creates a DockerHub repository
4. SSHs into the VPS to create a deploy user, deploy path, and SSH keypair
5. Writes a Caddy reverse proxy config and reloads Caddy
6. Creates DNS A and www CNAME records
7. Sets GitHub Actions secrets (namespaced per environment)
8. Generates GitHub Actions workflow files in `.github/workflows/`
9. Saves the project to the database
