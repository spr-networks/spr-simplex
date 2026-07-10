# spr-simplex

Run a self-hosted [SimpleX](https://simplex.chat) messaging relay —
`smp-server` from
[simplex-chat/simplexmq](https://github.com/simplex-chat/simplexmq) — as an
[SPR](https://github.com/spr-networks/super) plugin. Your SimpleX clients get
a private SMP relay on your own router instead of depending on public relays.

The plugin ships the official upstream `smp-server` release binary (pinned by
version + sha256), initializes and supervises it, and adds a small REST API +
web UI (embedded in the SPR interface under *Plugins*) with a guided
first-run: the server identity is generated automatically and the UI reveals
the one thing you need — your copyable `smp://` relay address.

## Features

- **smp-server v6.5.0**, the official Ubuntu 24.04 release binaries for
  x86-64 and aarch64, pinned by sha256; reproducible container build
- **Automatic first-run init** — `smp-server init` runs once, generating the
  offline CA, TLS certificate and fingerprint under
  `/state/plugins/spr-simplex`; the server identity (fingerprint) survives
  container rebuilds
- **Copyable relay address** — `smp://<fingerprint>@<container-ip>`, ready to
  paste into the SimpleX app (Network & servers → SMP servers)
- **Settings** — port, optional password-protected queue creation (the SMP
  server password, redacted on every read), store-log persistence and daily
  stats toggles (both off by default); changes rewrite `smp-server.ini` and
  restart the daemon
- **Topology** — contributes the relay as a service node to SPR's topology
  view (`HasTopology` + `GET /topology`)
- **No host ports.** The relay listens on the container IP `:5223` on the
  plugin's own docker bridge (`spr-simplex`); the management API is only
  reachable over the SPR plugin unix socket

## How it integrates with SPR

SPR proxies `/plugins/spr-simplex/…` to the plugin's unix socket at
`/state/plugins/spr-simplex/socket` and embeds the UI (served from the same
socket) as an iframe under **Plugins → spr-simplex**. The SMP relay itself is
only exposed on the `spr-simplex` docker bridge; SPR policies and the
`simplex` device group decide who can reach it.

## Exposing the relay to clients

`smp-server` binds `CONTAINER_IP:5223` (TLS) on the `spr-simplex` bridge. Two
ways to let SimpleX clients reach it:

1. **LAN-only (default).** The plugin interface carries the `lan` policy
   (plus the `simplex` group), so devices on the SPR LAN can reach
   `CONTAINER_IP:5223` directly. Copy the address from the UI and add it in
   the SimpleX app while on your network. Messages *to* you flow through your
   relay whenever the sender can reach it — which with this setup means while
   your devices are on the LAN.

2. **Internet exposure (optional, documented only).** To serve roaming
   devices and let contacts reach your queues from anywhere, add an SPR port
   forward: in the SPR UI, **Firewall → Port Forwarding**, forward WAN TCP
   `5223` to `CONTAINER_IP:5223`. Then use your public IP or DDNS name in the
   address: `smp://<fingerprint>@your.ddns.name`. SimpleX clients
   authenticate the server by its certificate **fingerprint**, not its
   hostname, so the address keeps working no matter what name or IP you put
   after the `@`. If you expose the relay publicly, set a **queue creation
   password** in the UI so strangers can't create queues on it (message
   *delivery* to existing queues stays open — that is how contacts reach
   you).

The container also has `wan`+`dns` egress so the relay can forward messages
to other SMP relays (SimpleX "private message routing", where your relay
proxies your sends so destination relays never see your IP).

## Install (UI)

In the SPR UI: **Plugins → + New Plugin** and enter this repository's GitHub
URL (e.g. `https://github.com/USER/spr-simplex`). SPR clones the repo, builds
the container and starts the plugin. The `plugin.json` `NetworkCapabilities`
register the `spr-simplex` interface with the `lan`, `wan` and `dns` policies
and the `simplex` group automatically.

## Install (CLI)

```sh
git clone https://github.com/USER/spr-simplex
cd spr-simplex
./install.sh    # prompts for SUPERDIR and an SPR API token
```

`install.sh` writes the API token, builds and starts the container, and
registers the container IP with SPR's firewall
(`PUT /firewall/custom_interface`, policies `lan wan dns`).

## API

All endpoints are served over the plugin unix socket and reachable (with SPR
auth) at `/plugins/spr-simplex/<path>`.

| Method | Path | Description |
| --- | --- | --- |
| GET | `/status` | Daemon state, uptime, version, fingerprint, address, port, toggles |
| GET | `/address` | The copyable relay address: `{Address, Fingerprint, Host, Port, QueuePasswordSet}` |
| GET | `/config` | Plugin configuration; the queue password is redacted to `QueuePasswordSet` |
| PUT | `/config` | Validate + save config, rewrite `smp-server.ini`, restart smp-server |
| POST | `/restart` | Restart the smp-server daemon |
| GET | `/topology` | Topology contribution (root anchor + relay service node) for SPR's topology view |

`PUT /config` body:

```json
{
  "Port": 5223,
  "StoreLog": false,
  "DailyStats": false,
  "QueuePassword": "",
  "ClearQueuePassword": false
}
```

`QueuePassword` semantics: empty keeps the stored password, non-empty
replaces it (8–128 printable ASCII characters; no whitespace, `@`, `:` or
`/`), `ClearQueuePassword: true` removes it. The password is never echoed
back by any endpoint.

## Configuration reference

`/configs/plugins/spr-simplex/config.json` (managed via the UI / API, mode
0600):

| Field | Default | Meaning |
| --- | --- | --- |
| `Port` | `5223` | TCP port smp-server listens on (`[TRANSPORT] port`). 5223 is the SMP default and is omitted from the address |
| `StoreLog` | `false` | Append-only store log so queues + undelivered messages survive restarts (`[STORE_LOG] enable/restore_messages`) |
| `DailyStats` | `false` | Daily aggregate statistics CSV (`[STORE_LOG] log_stats`) |
| `QueuePassword` | `""` | SMP basic auth for *creating* queues (`[AUTH] create_password`). Empty = open queue creation |

The plugin owns these keys in `/etc/opt/simplex/smp-server.ini` (a bind mount
under `/state/plugins/spr-simplex/smp/etc`) and rewrites them before every
daemon start; the rest of the generated ini is preserved. `smp-server init`
runs exactly once per server identity — it is never re-run while a
fingerprint exists, because init wipes the config dir and a new fingerprint
would strand every client.

## Security model

- **No published host ports**; `network_mode: host` is not used. The only
  listeners are the plugin unix socket (0770) and smp-server on the container
  IP `:5223` on the dedicated `spr-simplex` bridge, gated by SPR
  policies/groups (`lan` so LAN devices can use the relay, `wan`/`dns` egress
  for SMP private message routing).
- **No extra capabilities** (`cap_add` empty — smp-server is a plain
  userspace process), no devices,
  `security_opt: no-new-privileges:true`.
- **Secrets**: the CA key, TLS key, fingerprint and ini live on bind mounts
  under `/state/plugins/spr-simplex` (dirs 0700, files 0600, enforced on
  every start). The queue creation password is stored in the plugin config
  (0600) and **redacted on every read** — the API only ever reports
  `QueuePasswordSet: true`.
- **Input validation**: port and password are allow-list validated
  server-side before they touch `smp-server.ini`; the daemon and init are
  invoked with fixed argv arrays — no shell interpolation anywhere.
- The embedded web server, websockets and control port of smp-server stay
  disabled; the ini rewrite pins them off.

## Reproducible builds

All build inputs are pinned in `reproducible.env`: base images by digest, the
Go toolchain by version + sha256 (amd64 and arm64), Ubuntu packages from a
dated `snapshot.ubuntu.com` snapshot, and `smp-server` by release version +
sha256 of each architecture's official binary (upstream publishes no checksum
files, so the pins were computed by downloading and hashing the release
assets).

- `./build_docker_compose.sh` — reproducible local build (buildx +
  `rewrite-timestamp`, pins injected as build args)
- `./update-pins.sh` — re-resolve every pin (image digests, latest Go patch
  release + checksums, latest stable simplexmq release + binary hashes) and
  sync the Dockerfile ARG defaults

## Upstream

- [simplex-chat/simplexmq](https://github.com/simplex-chat/simplexmq) —
  AGPL-3.0 license. This plugin runs the unmodified official `smp-server`
  release binary and records the upstream source link in the server's
  `[INFORMATION]` section; it is not affiliated with SimpleX Chat Ltd.
- Wishlist context: [spr-networks/super#341](https://github.com/spr-networks/super/issues/341)

## License

MIT — see [LICENSE](LICENSE). The bundled `smp-server` binary remains under
upstream's AGPL-3.0.
