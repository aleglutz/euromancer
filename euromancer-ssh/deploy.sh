#!/bin/sh
# euromancer-ssh → Oracle VPS: build, sync content, restart.
# New post = commit to ../archive + ./deploy.sh
set -e
HOST=ubuntu@152.70.18.245
SSH="ssh -p 2222"
BIN=$(mktemp -t euromancer-ssh)

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$BIN" .
rsync -az --delete --exclude='.DS_Store' --rsync-path="sudo rsync" -e "$SSH" ../archive "$HOST":/opt/euromancer/
rsync -az --delete --exclude='.DS_Store' --rsync-path="sudo rsync" -e "$SSH" ../assets/images "$HOST":/opt/euromancer/assets/
rsync -az --rsync-path="sudo rsync" -e "$SSH" "$BIN" "$HOST":/opt/euromancer/euromancer-ssh
$SSH "$HOST" 'sudo chown -R euromancer:euromancer /opt/euromancer && sudo chmod 755 /opt/euromancer/euromancer-ssh && sudo systemctl restart euromancer && systemctl is-active euromancer'
rm -f "$BIN"
echo "deployed — ssh 152.70.18.245"
