#!/usr/bin/env bash
# peon.sh — bootstrap the peon sudo deploy user on a fresh Ubuntu LTS VPS
# Run as root: scp peon.sh root@your-vps:/root && ssh root@your-vps 'bash /root/peon.sh'

set -euo pipefail

PEON_USER="peon"
PEON_HOME="/home/${PEON_USER}"
PEON_SSH_DIR="${PEON_HOME}/.ssh"
PEON_KEY="${PEON_SSH_DIR}/id_ed25519"
SUDOERS_FILE="/etc/sudoers.d/${PEON_USER}"

# ── Colour helpers ────────────────────────────────────────────────────────────
green() { echo -e "\033[0;32m✓ $*\033[0m"; }
yellow() { echo -e "\033[0;33m→ $*\033[0m"; }
red() { echo -e "\033[0;31m✗ $*\033[0m"; }

# ── Must run as root ──────────────────────────────────────────────────────────
if [[ "${EUID}" -ne 0 ]]; then
  red "This script must be run as root."
  exit 1
fi

# ── Ensure docker is installed ────────────────────────────────────────────────
if ! command -v docker &>/dev/null; then
  yellow "Docker not found. Installing Docker..."
  apt-get update -qq
  apt-get install -y -qq ca-certificates curl gnupg lsb-release
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
    | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg
  echo \
    "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
    https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" \
    > /etc/apt/sources.list.d/docker.list
  apt-get update -qq
  apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-compose-plugin
  green "Docker installed."
else
  green "Docker already installed."
fi

# ── Create peon user if not exists ───────────────────────────────────────────
if id "${PEON_USER}" &>/dev/null; then
  green "User '${PEON_USER}' already exists, skipping creation."
else
  yellow "Creating user '${PEON_USER}'..."
  useradd \
    --create-home \
    --home-dir "${PEON_HOME}" \
    --shell /bin/bash \
    --system \
    "${PEON_USER}"
  green "User '${PEON_USER}' created."
fi

# ── Add peon to docker group ─────────────────────────────────────────────────
if groups "${PEON_USER}" | grep -q "\bdocker\b"; then
  green "'${PEON_USER}' already in docker group."
else
  yellow "Adding '${PEON_USER}' to docker group..."
  usermod -aG docker "${PEON_USER}"
  green "'${PEON_USER}' added to docker group."
fi

# ── Passwordless sudo ─────────────────────────────────────────────────────────
if [[ -f "${SUDOERS_FILE}" ]]; then
  green "Sudoers entry already exists."
else
  yellow "Granting '${PEON_USER}' passwordless sudo..."
  echo "${PEON_USER} ALL=(ALL) NOPASSWD:ALL" > "${SUDOERS_FILE}"
  chmod 0440 "${SUDOERS_FILE}"
  # Validate the sudoers file before continuing
  if ! visudo -cf "${SUDOERS_FILE}"; then
    red "Sudoers file validation failed. Removing."
    rm "${SUDOERS_FILE}"
    exit 1
  fi
  green "Passwordless sudo granted."
fi

# ── SSH keypair ───────────────────────────────────────────────────────────────
mkdir -p "${PEON_SSH_DIR}"
chmod 700 "${PEON_SSH_DIR}"
chown "${PEON_USER}:${PEON_USER}" "${PEON_SSH_DIR}"

if [[ -f "${PEON_KEY}" ]]; then
  green "SSH keypair already exists, skipping generation."
else
  yellow "Generating ed25519 SSH keypair for '${PEON_USER}'..."
  ssh-keygen -t ed25519 -f "${PEON_KEY}" -N "" -C "${PEON_USER}@$(hostname)"
  chown "${PEON_USER}:${PEON_USER}" "${PEON_KEY}" "${PEON_KEY}.pub"
  chmod 600 "${PEON_KEY}"
  chmod 644 "${PEON_KEY}.pub"
  green "SSH keypair generated."
fi

# ── Authorize the public key ──────────────────────────────────────────────────
AUTHORIZED_KEYS="${PEON_SSH_DIR}/authorized_keys"
PUBLIC_KEY=$(cat "${PEON_KEY}.pub")

if [[ -f "${AUTHORIZED_KEYS}" ]] && grep -qF "${PUBLIC_KEY}" "${AUTHORIZED_KEYS}"; then
  green "Public key already in authorized_keys."
else
  yellow "Adding public key to authorized_keys..."
  echo "${PUBLIC_KEY}" >> "${AUTHORIZED_KEYS}"
  chown "${PEON_USER}:${PEON_USER}" "${AUTHORIZED_KEYS}"
  chmod 600 "${AUTHORIZED_KEYS}"
  green "Public key added."
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "══════════════════════════════════════════════════════"
echo "  peon setup complete"
echo "══════════════════════════════════════════════════════"
echo "  User:        ${PEON_USER}"
echo "  Home:        ${PEON_HOME}"
echo "  Sudo:        passwordless"
echo "  Docker:      yes"
echo "  Public key:  ${PEON_KEY}.pub"
echo "══════════════════════════════════════════════════════"
echo ""
echo "Private key (save this to your local machine):"
echo "──────────────────────────────────────────────"
cat "${PEON_KEY}"
echo "──────────────────────────────────────────────"
echo ""
echo "Add the above private key to ~/.dotfiles/.env as:"
echo "  PEON_SSH_KEY=\"\$(cat /path/to/saved/key)\""
echo ""
