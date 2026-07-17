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

KRUN_MAC="02:53:50:52:4b:0f"
KRUN_TAP="ksimplex0"
curl --fail-with-body --silent --show-error "http://127.0.0.1/device?identity=${KRUN_MAC}" \
  -H "Authorization: Bearer ${SPR_API_TOKEN}" -H "Content-Type: application/json" \
  -X PUT --data-raw "{\"MAC\":\"${KRUN_MAC}\",\"Name\":\"spr-simplex\",\"Policies\":[\"wan\",\"dns\"],\"Groups\":[\"simplex\"]}" >/dev/null
if ! sudo nft get element inet filter dhcp_access "{ \"${KRUN_TAP}\" . ${KRUN_MAC} }" >/dev/null 2>&1; then
  sudo nft add element inet filter dhcp_access "{ \"${KRUN_TAP}\" . ${KRUN_MAC} : accept }"
fi

docker compose -f docker-compose-krun.yml build
docker compose -f docker-compose-krun.yml up -d

CONTAINER_IP=
for _ in $(seq 1 30); do
  CONTAINER_IP="$(jq -r --arg mac "$KRUN_MAC" '.[$mac].RecentIP // empty' "$SUPERDIR/state/public/devices-public.json")"
  [ -n "$CONTAINER_IP" ] && break
  sleep 1
done
[ -n "$CONTAINER_IP" ] || { echo "spr-simplex did not obtain an SPR DHCP lease" >&2; exit 1; }
API=127.0.0.1

# Register the plugin's bridge interface with the SPR firewall:
# The simplex group gates access to the relay on ${CONTAINER_IP}:5223;
# wan+dns lets the relay forward messages to other SMP relays (SimpleX
# private message routing).
curl "http://${API}/firewall/custom_interface" \
-H "Authorization: Bearer ${SPR_API_TOKEN}" \
-X 'PUT' \
--data-raw "{\"SrcIP\":\"${CONTAINER_IP}\",\"Interface\":\"${KRUN_TAP}\",\"Policies\":[\"wan\",\"dns\"],\"Groups\":[\"simplex\"]}"

docker compose -f docker-compose-krun.yml restart

echo ""
echo "spr-simplex is up. Open Plugins -> spr-simplex in the SPR UI to copy"
echo "your relay address (smp://<fingerprint>@${CONTAINER_IP}) into the"
echo "SimpleX app under Network & servers -> SMP servers."
