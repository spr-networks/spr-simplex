#!/bin/bash
# Command line install alternative to the UI (+ New Plugin)
echo "Please enter your SPR path (/home/spr/super/)"
read -r SUPERDIR

if [ -z "$SUPERDIR" ]; then
    SUPERDIR="/home/spr/super/"
fi

export SUPERDIR

echo "Please enter your SPR API token:"
read -r SPR_API_TOKEN

if [ -z "$SPR_API_TOKEN" ]; then
  echo "need api token, generate one on the auth keys page"
  exit 1
fi

mkdir -p "$SUPERDIR/configs/plugins/spr-simplex"

# Token used by SPR to authorize the plugin (InstallTokenPath)
printf '%s' "$SPR_API_TOKEN" > "$SUPERDIR/configs/plugins/spr-simplex/api-token"
chmod 600 "$SUPERDIR/configs/plugins/spr-simplex/api-token"

docker compose build
docker compose up -d

CONTAINER_IP=$(docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "spr-simplex")
API=127.0.0.1

# Register the plugin's bridge interface with the SPR firewall:
# The simplex group gates access to the relay on ${CONTAINER_IP}:5223;
# wan+dns lets the relay forward messages to other SMP relays (SimpleX
# private message routing).
curl "http://${API}/firewall/custom_interface" \
-H "Authorization: Bearer ${SPR_API_TOKEN}" \
-X 'PUT' \
--data-raw "{\"SrcIP\":\"${CONTAINER_IP}\",\"Interface\":\"spr-simplex\",\"Policies\":[\"wan\",\"dns\"],\"Groups\":[\"simplex\"]}"

docker compose restart

echo ""
echo "spr-simplex is up. Open Plugins -> spr-simplex in the SPR UI to copy"
echo "your relay address (smp://<fingerprint>@${CONTAINER_IP}) into the"
echo "SimpleX app under Network & servers -> SMP servers."
