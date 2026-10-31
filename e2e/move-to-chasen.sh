#!/usr/bin/env bash
# The test of moving a server of the older installer to Chasen: it sets one
# up with the released lognorth binary, as the install line did before, then
# runs the steps of the docs with the image of this commit, and checks that
# the account, the session key, and the domain stay, and that Chasen updates
# LogNorth each night from then on.
#
# It needs Docker, sudo, and the ports 80 and 443, and it changes the
# machine: run it on a CI runner or in a throwaway VM, not on your computer.
#   e2e/move-to-chasen.sh
set -euo pipefail
cd "$(dirname "$0")/.."

registry=localhost:5000 # a registry with no login, like Docker Hub for a public image
image=$registry/lognorth:e2e
host=lognorth.localhost
email=owner@example.com
password=correct-horse-battery

pass() { echo "ok   - $*"; }
fail() { echo "FAIL - $*" >&2; exit 1; }
web() { curl -s -H "Host: $host" "$@"; }
post() { web -o /dev/null -w '%{http_code} %{redirect_url}' -H "Origin: http://$host" "$@"; }
login() { post -d "email=$email" --data-urlencode "password=$1" http://127.0.0.1/login; }
# The private key of the container that runs: lognorth, or lognorth-next after a swap.
key() { docker exec "$(docker ps --format '{{.Names}}' | grep -m1 -E '^lognorth(-next)?$')" printenv PRIVATE_KEY; }

docker rm -f lognorth-e2e-registry >/dev/null 2>&1 || true
docker run -d --name lognorth-e2e-registry -p 127.0.0.1:5000:5000 registry:2 >/dev/null
trap 'docker rm -f lognorth-e2e-registry >/dev/null 2>&1 || true' EXIT

echo "--- the image of this commit"
docker build -q -t "$image" . >/dev/null
docker push -q "$image" >/dev/null

echo "--- a server of the older installer, with the released image"
case "$(uname -m)" in x86_64) arch=amd64 ;; aarch64) arch=arm64 ;; *) fail "no build for $(uname -m)" ;; esac
# v0.28.1 is the last release with the binary of the older installer.
curl -fsSL -o /tmp/lognorth "https://github.com/karloscodes/lognorth/releases/download/v0.28.1/lognorth-linux-$arch"
sudo install -m 755 /tmp/lognorth /usr/local/bin/lognorth
printf '%s\ny\n' "$host" | sudo lognorth install >/tmp/old-install.log 2>&1 || { cat /tmp/old-install.log; fail "the older installer failed"; }
for _ in $(seq 30); do [[ "$(web -o /dev/null -w '%{http_code}' http://127.0.0.1/_health)" == 200 ]] && break; sleep 2; done
created="$(post -d "email=$email" -d "password=$password" -d "confirm_password=$password" http://127.0.0.1/setup)"
[[ "$created" == 302" http://127.0.0.1/" ]] || fail "the setup on the older install did not create the account: $created"
[[ "$(login "$password")" == 302" http://127.0.0.1/" ]] || fail "the older install does not log in"
[[ -f /etc/cron.d/lognorth-update ]] || fail "the older installer made no nightly update"
old_key="$(key)"
pass "the older installer runs LogNorth, and its account logs in"

echo "--- the move, as the docs say"
curl -fsSL https://chasenhq.com/server | sudo sh >/dev/null
sudo chasen-server setup >/dev/null
sudo chasen-server adopt lognorth
sudo rm /etc/cron.d/lognorth-update /usr/local/bin/lognorth
sudo LOGNORTH_IMAGE="$image" bash install.sh

[[ "$(web -o /dev/null -w '%{http_code}' http://127.0.0.1/up)" == 200 ]] || fail "/up does not answer after the move"
pass "/up answers after the move"
[[ "$(login "$password")" == 302" http://127.0.0.1/" ]] || fail "the account does not log in after the move"
pass "the account stays"
status="$(sudo chasen-server status lognorth)" # not in a pipe: grep -q stops early, and pipefail fails the status
[[ -n "$old_key" && "$(key)" == "$old_key" ]] || fail "the session key changed: every login ends"
pass "the session key stays, so the logins stay"
grep -q "Updates:  each night" <<<"$status" || fail "the nightly updates of Chasen are off"
[[ ! -f /etc/cron.d/lognorth-update ]] || fail "the nightly update of the older installer is still there"
pass "Chasen updates it each night, and the older nightly update is gone"
grep -qE "^URL: +.*$host" <<<"$status" || fail "the domain changed: $status"
pass "the domain stays"

echo "--- the install line on the moved server: an update"
again="$(sudo LOGNORTH_IMAGE="$image" bash install.sh 2>&1)" || { echo "$again"; fail "the install line failed on the moved server"; }
grep -q "LogNorth is up to date" <<<"$again" || fail "the install line did not update the moved server: $again"
[[ "$(login "$password")" == 302" http://127.0.0.1/" ]] || fail "the login fails after the update"
pass "the install line updates the moved server"

echo "All checks passed."
