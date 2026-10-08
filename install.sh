#!/bin/bash
# Installs LogNorth on this server with Chasen (https://chasenhq.com):
# HTTPS, live backups of the database, and an update each night.
#   curl -fsSL lognorth.com/install | sudo bash -s logs.example.com
#
# The domain is the first argument, or LOGNORTH_DOMAIN. Without either, it asks.
# LOGNORTH_IMAGE deploys another image than karloscodes/lognorth:latest,
# for a test. Run it again to update LogNorth now.
#
# Each release attaches this file, and lognorth.com/install points to the
# newest one: a change here reaches people only after the tests of a release.
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'
image="${LOGNORTH_IMAGE:-karloscodes/lognorth:latest}"

fail() { echo -e "${RED}Error: $*${NC}" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || fail "run it as root: curl -fsSL lognorth.com/install | sudo bash -s logs.example.com"
[ "$(uname -s)" = Linux ] || fail "LogNorth installs on a Linux server. From your computer, deploy it with Chasen: https://lognorth.com/docs/installation/"

# A server that the older installer set up keeps it: it updates itself each
# night. This script never touches a LogNorth that runs.
if [ -f /etc/cron.d/lognorth-update ] || grep -qs '^ *lognorth:' /etc/matcha/config.yml; then
	echo "This server runs LogNorth from the older installer. It keeps updating itself each night,"
	echo "and its commands stay: lognorth update, status, reset-password."
	echo "Nothing changed. To move it to Chasen: https://lognorth.com/docs/move-to-chasen/"
	exit 0
fi

# LogNorth runs on Chasen already, also after a move from the older
# installer: deploy the newest image, keep the settings, and update it each
# night from now on.
if command -v chasen-server >/dev/null 2>&1 && chasen-server list 2>/dev/null | grep -q '^lognorth '; then
	echo "Updating LogNorth to the newest $image..."
	printf '{"image":"%s","keep_settings":true,"auto_update":true,"env":{}}\n' "$image" | chasen-server deploy lognorth latest
	echo -e "${GREEN}LogNorth is up to date.${NC}"
	exit 0
fi

domain="${1:-${LOGNORTH_DOMAIN:-}}"
if [ -z "$domain" ]; then
	[ -r /dev/tty ] || fail "no domain: curl -fsSL lognorth.com/install | sudo bash -s logs.example.com"
	read -r -p "Domain for LogNorth (e.g. logs.example.com): " domain </dev/tty
fi
domain="$(echo "$domain" | tr '[:upper:]' '[:lower:]')"
[[ "$domain" =~ ^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$ && "$domain" == *.* ]] || fail "\"$domain\" is not a domain"

echo "Installing Chasen..."
curl -fsSL https://chasenhq.com/server | sh
chasen-server setup >/dev/null # it installs Docker when the server has none

echo "Deploying LogNorth at $domain..."
printf '{"image":"%s","domain":"%s","auto_update":true,"env":{}}\n' "$image" "$domain" | chasen-server deploy lognorth latest

echo
echo -e "${GREEN}LogNorth runs at https://$domain${NC}"
echo "  Point an A record for $domain to this server: HTTPS comes on the first request."
echo "  Open it now and create your account: until you do, anyone who opens it can."
echo "  It updates itself each night, with a backup of the database first."
echo
echo "Manage it from your computer:"
echo "  curl -fsSL https://chasenhq.com/cli | sh"
echo "  chasen add server root@<this server>"
echo "  chasen -a lognorth status        # also: logs, backups, restore, rollback"
