import React, { useEffect, useRef, useState } from 'react'
import {
  api,
  useAlert,
  timeAgo,
  Page,
  ListHeader,
  Card,
  SectionHeader,
  StatTile,
  KeyVal,
  StatusDot,
  Toggle,
  TextField,
  ModalConfirm,
  Loading,
  EmptyState,
  Badge,
  BadgeText,
  Box,
  Button,
  ButtonText,
  HStack,
  Spinner,
  Text,
  VStack
} from '@spr-networks/plugin-ui'

const BASE = `/plugins/${api.pluginURI() || 'spr-simplex'}`

const fmtUptime = (secs) => {
  if (!secs || secs < 0) return '—'
  const d = Math.floor(secs / 86400)
  const h = Math.floor((secs % 86400) / 3600)
  const m = Math.floor((secs % 3600) / 60)
  if (d) return `${d}d ${h}h`
  if (h) return `${h}h ${m}m`
  if (m) return `${m}m`
  return `${Math.floor(secs)}s`
}

// "SMP server v6.5.0 ..." -> "v6.5.0" for the stat tile; details keep the full banner
const shortVersion = (v) => {
  const m = (v || '').match(/v\d+[\w.-]*/)
  return m ? m[0] : v || '—'
}

const validatePort = (portStr) => {
  if (!String(portStr).trim()) return 'Port is required'
  const n = Number(portStr)
  if (!Number.isInteger(n) || n < 1 || n > 65535)
    return 'Enter a whole number between 1 and 65535'
  return ''
}

const validatePassword = (pw) => {
  if (!pw) return ''
  if (pw.length < 8 || pw.length > 128) return 'Use 8–128 characters'
  if (!/^[\x21-\x7e]+$/.test(pw))
    return 'Printable ASCII only, no spaces'
  if (/[@:/]/.test(pw))
    return "Cannot contain '@', ':' or '/' (they break the smp:// address)"
  return ''
}

const CopyButton = ({ value, label = 'Copy', onCopied, onFailed }) => (
  <Button
    size="xs"
    variant="outline"
    isDisabled={!value}
    onPress={() => {
      if (value && navigator?.clipboard?.writeText) {
        navigator.clipboard.writeText(value).then(onCopied).catch(onFailed)
      } else {
        onFailed()
      }
    }}
  >
    <ButtonText>{label}</ButtonText>
  </Button>
)

export default function Plugin() {
  const alert = useAlert()
  const [loading, setLoading] = useState(true)
  const [unreachable, setUnreachable] = useState(false)
  const [status, setStatus] = useState(null)
  const [address, setAddress] = useState(null)
  const [config, setConfig] = useState(null)

  // settings form
  const [portStr, setPortStr] = useState('')
  const [storeLog, setStoreLog] = useState(false)
  const [dailyStats, setDailyStats] = useState(false)
  const [newPassword, setNewPassword] = useState('')
  const [saving, setSaving] = useState(false)

  // deliberate actions
  const [showRestart, setShowRestart] = useState(false)
  const [restarting, setRestarting] = useState(false)
  const [showRemovePw, setShowRemovePw] = useState(false)

  const formInit = useRef(false)

  const refresh = () => {
    return Promise.allSettled([
      api.get(`${BASE}/status`),
      api.get(`${BASE}/address`),
      api.get(`${BASE}/config`)
    ]).then(([s, a, c]) => {
      if (s.status === 'fulfilled') {
        setStatus(s.value)
        setUnreachable(false)
      } else {
        setUnreachable(true)
      }
      if (a.status === 'fulfilled') setAddress(a.value)
      if (c.status === 'fulfilled' && c.value) {
        setConfig(c.value)
        // seed the form once; later refreshes must not clobber edits
        if (!formInit.current) {
          setPortStr(String(c.value.Port))
          setStoreLog(!!c.value.StoreLog)
          setDailyStats(!!c.value.DailyStats)
          formInit.current = true
        }
      }
      setLoading(false)
    })
  }

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 10000)
    return () => clearInterval(t)
  }, [])

  // during first-run identity generation, poll faster so the reveal is quick
  useEffect(() => {
    if (!status || status.InitDone) return
    const t = setInterval(refresh, 3000)
    return () => clearInterval(t)
  }, [status && status.InitDone])

  const portError = validatePort(portStr)
  const pwError = validatePassword(newPassword)
  const dirty =
    !!config &&
    (portStr !== String(config.Port) ||
      storeLog !== !!config.StoreLog ||
      dailyStats !== !!config.DailyStats ||
      newPassword !== '')

  const resetForm = () => {
    if (!config) return
    setPortStr(String(config.Port))
    setStoreLog(!!config.StoreLog)
    setDailyStats(!!config.DailyStats)
    setNewPassword('')
  }

  const save = () => {
    if (portError || pwError || !dirty || saving) return
    setSaving(true)
    api
      .put(`${BASE}/config`, {
        Port: Number(portStr),
        StoreLog: storeLog,
        DailyStats: dailyStats,
        QueuePassword: newPassword,
        ClearQueuePassword: false
      })
      .then((c) => {
        setConfig(c)
        setNewPassword('')
        alert.success('Saved — relay restarted')
        refresh()
      })
      .catch((err) => alert.error('Failed to save settings', err))
      .finally(() => setSaving(false))
  }

  const removePassword = () => {
    // applies only the password removal — saved settings stay as-is
    api
      .put(`${BASE}/config`, {
        Port: config.Port,
        StoreLog: !!config.StoreLog,
        DailyStats: !!config.DailyStats,
        QueuePassword: '',
        ClearQueuePassword: true
      })
      .then((c) => {
        setConfig(c)
        setNewPassword('')
        alert.success('Queue creation password removed')
        refresh()
      })
      .catch((err) => alert.error('Failed to remove password', err))
  }

  const restart = () => {
    setRestarting(true)
    api
      .post(`${BASE}/restart`)
      .then(() => {
        alert.success('Relay restarted')
        refresh()
      })
      .catch((err) => alert.error('Restart failed', err))
      .finally(() => setRestarting(false))
  }

  const copied = () => alert.success('Copied to clipboard')
  const copyFailed = () =>
    alert.warning('Copy failed — select the text and copy manually')

  if (loading) {
    return (
      <Page>
        <Loading />
      </Page>
    )
  }

  if (unreachable || !status) {
    return (
      <Page>
        <ListHeader
          title="SimpleX relay"
          description="Self-hosted SMP server for SimpleX Chat"
        />
        <Card>
          <EmptyState
            title="Backend unreachable"
            description="The spr-simplex plugin API did not respond. If the plugin was just installed or updated, the container may still be starting."
          >
            <Button
              size="sm"
              onPress={() => {
                setLoading(true)
                refresh()
              }}
            >
              <ButtonText>Retry</ButtonText>
            </Button>
          </EmptyState>
        </Card>
      </Page>
    )
  }

  const running = !!status.Running
  const initDone = !!status.InitDone
  const smpAddress = (address && address.Address) || status.Address || ''

  // ---- first run: identity is being generated ----
  if (!initDone) {
    return (
      <Page>
        <ListHeader
          title="SimpleX relay"
          description="Self-hosted SMP server for SimpleX Chat"
          status="Setting up"
          statusAction="warning"
        />
        <Card>
          <SectionHeader title="Setting up your relay" />
          <VStack space="md">
            <HStack space="sm" alignItems="center">
              <Spinner size="small" />
              <Text size="sm">
                Generating the server identity — an offline CA certificate and
                its fingerprint. This normally takes a few seconds.
              </Text>
            </HStack>
            <VStack space="xs">
              <Text size="sm" color="$muted500">
                1. The relay creates its TLS certificate and fingerprint (kept
                in the plugin state dir, never leaves the router).
              </Text>
              <Text size="sm" color="$muted500">
                2. Your relay address appears here — one copyable smp:// line.
              </Text>
              <Text size="sm" color="$muted500">
                3. Paste it into the SimpleX app under Network &amp; servers →
                SMP servers, and your messages stop relying on public relays.
              </Text>
            </VStack>
          </VStack>
        </Card>
      </Page>
    )
  }

  return (
    <Page>
      <ListHeader
        title="SimpleX relay"
        description="Self-hosted SMP server for SimpleX Chat"
        status={running ? 'Running' : 'Stopped'}
        statusAction={running ? 'success' : 'muted'}
      >
        <Button
          size="sm"
          variant="outline"
          isDisabled={restarting}
          onPress={() => setShowRestart(true)}
        >
          <ButtonText>{restarting ? 'Restarting…' : 'Restart'}</ButtonText>
        </Button>
      </ListHeader>

      {/* ---- hero: state + the copyable address ---- */}
      <Card>
        <SectionHeader
          title="Your relay address"
          right={<StatusDot online={running} />}
        />
        <VStack space="md">
          <HStack space="sm" alignItems="center" flexWrap="wrap">
            <Box
              flex={1}
              minWidth={260}
              px="$3"
              py="$2.5"
              borderRadius="$lg"
              borderWidth={1}
              borderColor="$muted200"
              bg="$backgroundContentLight"
              sx={{
                _dark: {
                  bg: '$backgroundContentDark',
                  borderColor: '$muted700'
                }
              }}
            >
              <Text
                size="sm"
                selectable
                sx={{ '@base': { fontFamily: 'monospace', wordBreak: 'break-all' } }}
              >
                {smpAddress || '—'}
              </Text>
            </Box>
            <CopyButton
              value={smpAddress}
              label="Copy address"
              onCopied={copied}
              onFailed={copyFailed}
            />
          </HStack>
          <Text size="sm" color="$muted500">
            In the SimpleX app: Network &amp; servers → SMP servers → Add
            server, paste this address, test and save. Devices in the SPR
            "simplex" group reach the relay while on your LAN; see the README
            to expose it to the internet with an SPR port forward.
          </Text>
          {status.QueuePasswordSet ? (
            <Text size="sm" color="$muted500">
              Queue creation is password-protected. On your own devices, add
              the password after the fingerprint:{' '}
              <Text size="sm" color="$muted500" sx={{ '@base': { fontFamily: 'monospace' } }}>
                smp://&lt;fingerprint&gt;:&lt;password&gt;@{status.Host || 'host'}
              </Text>
            </Text>
          ) : null}
        </VStack>
      </Card>

      {/* ---- operational numbers ---- */}
      <Card>
        <SectionHeader title="Status" />
        <HStack flexWrap="wrap" gap="$2">
          <StatTile label="State" value={running ? 'Running' : 'Stopped'} />
          <StatTile label="Uptime" value={running ? fmtUptime(status.UptimeSeconds) : '—'} />
          <StatTile label="Version" value={shortVersion(status.Version)} mono />
          <StatTile label="Port" value={String(status.Port || '—')} mono />
          <StatTile
            label="Queue creation"
            value={status.QueuePasswordSet ? 'Password required' : 'Open'}
          />
          <StatTile
            label="Persistence"
            value={status.StoreLog ? 'Store log on' : 'In-memory'}
          />
        </HStack>
        <VStack space="xs" mt="$3">
          <HStack space="md" alignItems="center" flexWrap="wrap">
            <Box flex={1} minWidth={240}>
              <KeyVal label="Fingerprint" value={status.Fingerprint || '—'} mono />
            </Box>
            <CopyButton
              value={status.Fingerprint}
              onCopied={copied}
              onFailed={copyFailed}
            />
          </HStack>
          <KeyVal
            label="Listening on"
            value={status.Host ? `${status.Host}:${status.Port}` : '—'}
            mono
          />
          <KeyVal
            label="Started"
            value={running ? timeAgo(status.StartedAt) || '—' : '—'}
          />
          <KeyVal label="Server build" value={status.Version || '—'} mono />
        </VStack>
      </Card>

      {/* ---- settings ---- */}
      <Card>
        <SectionHeader
          title="Settings"
          right={
            status.QueuePasswordSet ? (
              <Badge action="success" variant="outline" borderRadius="$full">
                <BadgeText>Password configured ✓</BadgeText>
              </Badge>
            ) : (
              <Badge action="muted" variant="outline" borderRadius="$full">
                <BadgeText>Open queue creation</BadgeText>
              </Badge>
            )
          }
        />
        <VStack space="md">
          <TextField
            label="Port"
            value={portStr}
            onChangeText={setPortStr}
            placeholder="5223"
            keyboardType="numeric"
            error={portError && dirty ? portError : ''}
            helper="TCP port the relay listens on, on the spr-simplex bridge. 5223 is the SMP default and is implied in the address."
          />
          <HStack justifyContent="space-between" alignItems="center">
            <VStack flex={1} pr="$4">
              <Text>Persist queues across restarts</Text>
              <Text size="xs" color="$muted500">
                Append-only store log for queues and undelivered messages.
                Recommended once contacts depend on this relay; off matches the
                upstream default.
              </Text>
            </VStack>
            <Toggle
              value={storeLog}
              onPress={() => setStoreLog(!storeLog)}
              label="Persist queues across restarts"
            />
          </HStack>
          <HStack justifyContent="space-between" alignItems="center">
            <VStack flex={1} pr="$4">
              <Text>Daily statistics</Text>
              <Text size="xs" color="$muted500">
                Aggregate daily usage counts to a CSV in the plugin state dir.
                No message content or addresses are logged.
              </Text>
            </VStack>
            <Toggle
              value={dailyStats}
              onPress={() => setDailyStats(!dailyStats)}
              label="Daily statistics"
            />
          </HStack>
          <TextField
            label={
              status.QueuePasswordSet
                ? 'Replace queue creation password'
                : 'Queue creation password'
            }
            value={newPassword}
            onChangeText={setNewPassword}
            placeholder={
              status.QueuePasswordSet ? 'Configured ✓ — enter to replace' : 'Not set'
            }
            secureTextEntry
            error={pwError}
            helper="Only clients that put this password in the server address can create queues here. 8–128 printable ASCII characters; no spaces, '@', ':' or '/'. Leave blank to keep the current setting."
          />
          {status.QueuePasswordSet ? (
            <HStack>
              <Button
                size="xs"
                variant="outline"
                action="negative"
                onPress={() => setShowRemovePw(true)}
              >
                <ButtonText>Remove password</ButtonText>
              </Button>
            </HStack>
          ) : null}
          <HStack
            justifyContent="space-between"
            alignItems="center"
            flexWrap="wrap"
            gap="$2"
          >
            <Text size="xs" color="$muted500" flex={1} minWidth={200}>
              Saving restarts the relay; connected clients reconnect
              automatically.
            </Text>
            <HStack space="sm">
              {dirty ? (
                <Button size="sm" variant="outline" action="secondary" onPress={resetForm}>
                  <ButtonText>Discard</ButtonText>
                </Button>
              ) : null}
              <Button
                size="sm"
                isDisabled={!dirty || saving || !!portError || !!pwError}
                onPress={save}
              >
                <ButtonText>{saving ? 'Saving…' : 'Save & apply'}</ButtonText>
              </Button>
            </HStack>
          </HStack>
        </VStack>
      </Card>

      <ModalConfirm
        isOpen={showRestart}
        onClose={() => setShowRestart(false)}
        onConfirm={restart}
        title="Restart the SMP relay?"
        message="Connected SimpleX clients disconnect briefly and reconnect automatically. Queued messages are kept only if 'Persist queues across restarts' is on."
        confirmText="Restart"
      />

      <ModalConfirm
        isOpen={showRemovePw}
        onClose={() => setShowRemovePw(false)}
        onConfirm={removePassword}
        title="Remove the queue creation password?"
        message="Anyone who can reach the relay (your LAN's simplex group, or the internet if you forwarded the port) will be able to create message queues on it. The relay restarts to apply."
        confirmText="Remove password"
        destructive
      />
    </Page>
  )
}
