#!/bin/bash
set -a
. /configs/base/config.sh
if [ -f /configs/spr-simplex/config.sh ]; then
    . /configs/spr-simplex/config.sh
fi
set +a

# smp-server config (CA key, TLS cert, fingerprint, ini) and store logs live
# on bind mounts under the plugin state dir — keep them owner-only.
mkdir -p /state/plugins/spr-simplex /etc/opt/simplex /var/opt/simplex
chmod 700 /etc/opt/simplex /var/opt/simplex

# smp-server is supervised by the plugin binary (initialized on first start,
# restarted on config change, watched for crashes); logs go to stdout/journald.
exec /simplex_plugin
