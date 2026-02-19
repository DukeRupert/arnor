# arnor

A unified infrastructure management CLI for managing web projects hosted on Hetzner Cloud with DNS via Porkbun or Cloudflare. Wraps three component tools — `shadowfax` (Porkbun DNS), `gwaihir` (Cloudflare DNS), and `fornost` (Hetzner Cloud) — into a single interface.

## Setup

### Prerequisites

- Go 1.25+
- [gh CLI](https://cli.github.com/) (for `project create`)
- `peon` user provisioned on target servers
- Environment variables in `~/.dotfiles/.env`:

```env
PORKBUN_API_KEY=...
PORKBUN_SECRET_KEY=...
CLOUDFLARE_API_TOKEN=...
HETZNER_API_TOKEN_PROD=...
HETZNER_API_TOKEN_DEV=...
PEON_SSH_KEY="-----BEGIN OPENSSH PRIVATE KEY-----
...
-----END OPENSSH PRIVATE KEY-----"
```

### Install

```bash
go build -o arnor .
```

### Initialize config

```bash
arnor config init
```

Queries your Hetzner projects and populates `~/.config/arnor/config.yaml` with discovered servers.

## Usage

### Config

```bash
arnor config init    # Auto-generate config from provider APIs
arnor config view    # Print current config
```

### Servers

```bash
arnor server list          # List all servers across Hetzner projects
arnor server view my-vps   # Show details for a specific server
```

### DNS

DNS provider is auto-detected from the domain's nameservers.

```bash
arnor dns list --domain example.com
arnor dns create --domain example.com --type A --content 1.2.3.4
arnor dns create --domain example.com --name www --type CNAME --content example.com
arnor dns delete --domain example.com --id 12345
```

### SSH Keys

```bash
arnor ssh list                                           # List keys across all projects
arnor ssh add --name my-key --key ~/.ssh/id_ed25519.pub --project prod
```

### Projects

```bash
arnor project list          # List all configured projects
arnor project view myclient # Show project details with environments
arnor project create        # Interactive wizard for full project setup
```

### Containers

```bash
arnor tui  # Navigate to "Containers" to view running Docker containers on any VPS
```

Connects via SSH as `peon` and displays all running containers with name, image, status, and port bindings in a scrollable card view.

### Project Create Workflow

`arnor project create` is the primary function — it replaces manual server setup with an interactive wizard that:

1. Looks up the server IP from Hetzner
2. Detects the DNS provider from nameservers
3. SSHs into the VPS to create a deploy user, deploy path, and SSH keypair
4. Writes a Caddy reverse proxy config and reloads Caddy
5. Creates DNS A and www CNAME records
6. Sets GitHub Actions secrets (namespaced per environment)
7. Generates GitHub Actions workflow files in `.github/workflows/`
8. Saves the project to config
