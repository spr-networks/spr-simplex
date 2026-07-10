package main

import (
	"strings"
	"testing"
)

// sampleIni mirrors the file `smp-server init -y --ip 172.18.0.2
// --no-password --source-code --disable-web` generates (v6.5.0, abridged but
// structurally faithful: prose comments, commented-out keys, multi-value
// port).
const sampleIni = `[INFORMATION]
# AGPLv3 license requires that you make any source code modifications
# available to the end users of the server.
source_code = https://github.com/simplex-chat/simplexmq

[STORE_LOG]
# The server uses memory or PostgreSQL database for persisting queue records.
# Use ` + "`enable = on`" + ` to use append-only log to preserve and restore queue records on restart.
# Log is compacted on start (deleted objects are removed).
enable = off

store_queues = memory

restore_messages = off

expire_messages_days = 21
expire_messages_on_start = on
expire_messages_on_send = off

# Log daily server statistics to CSV file
log_stats = off

[AUTH]
# Set new_queues option to off to completely prohibit creating new messaging queues.
new_queues = on

# Use create_password option to enable basic auth to create new messaging queues.
# The password should be used as part of server address in client configuration:
# smp://fingerprint:password@host1,host2
# create_password = password to create new queues and forward messages (any printable ASCII characters without whitespace, '@', ':' and '/')

# control_port_admin_password =
# control_port_user_password =

[TRANSPORT]
# Host is only used to print server address on start.
# You can specify multiple server ports.
host = 172.18.0.2
port = 5223,443
log_tls_errors = off
websockets = off
# control_port = 5224

[INACTIVE_CLIENTS]
disconnect = on
ttl = 21600
check_interval = 3600

[WEB]
static_path = /var/opt/simplex/www

# https = 443
# cert = /etc/opt/simplex/web.crt
# key = /etc/opt/simplex/web.key
`

func activeLines(ini string) map[string]bool {
	out := map[string]bool{}
	section := ""
	for _, line := range strings.Split(ini, "\n") {
		t := strings.TrimSpace(line)
		if s, ok := parseSectionHeader(t); ok {
			section = s
			continue
		}
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		out[section+"/"+t] = true
	}
	return out
}

func TestPatchIniReplacesActiveKey(t *testing.T) {
	got := patchIni(sampleIni, []IniOverride{
		{Section: "TRANSPORT", Key: "port", Value: "5223"},
	})
	lines := activeLines(got)
	if !lines["TRANSPORT/port = 5223"] {
		t.Fatalf("port line not rewritten:\n%s", got)
	}
	if lines["TRANSPORT/port = 5223,443"] {
		t.Fatalf("old port line still present")
	}
}

func TestPatchIniUncommentsCommentedKey(t *testing.T) {
	got := patchIni(sampleIni, []IniOverride{
		{Section: "AUTH", Key: "create_password", Value: "s3cretPW!"},
	})
	lines := activeLines(got)
	if !lines["AUTH/create_password = s3cretPW!"] {
		t.Fatalf("create_password not activated:\n%s", got)
	}
	// prose comment mentioning the key must survive untouched
	if !strings.Contains(got, "# Use create_password option to enable basic auth") {
		t.Fatalf("prose comment was clobbered")
	}
	// the commented placeholder must be gone (replaced in place)
	if strings.Contains(got, "password to create new queues and forward messages") {
		t.Fatalf("commented placeholder still present")
	}
}

func TestPatchIniDeleteRemovesActiveKey(t *testing.T) {
	withPw := patchIni(sampleIni, []IniOverride{
		{Section: "AUTH", Key: "create_password", Value: "s3cretPW!"},
	})
	got := patchIni(withPw, []IniOverride{
		{Section: "AUTH", Key: "create_password", Delete: true},
	})
	if strings.Contains(got, "s3cretPW!") {
		t.Fatalf("password still present after delete:\n%s", got)
	}
}

func TestPatchIniDeleteMissingKeyIsNoop(t *testing.T) {
	got := patchIni(sampleIni, []IniOverride{
		{Section: "PROXY", Key: "socks_proxy", Delete: true},
	})
	if got != sampleIni {
		t.Fatalf("delete of missing key modified content")
	}
}

func TestPatchIniInsertsMissingKeyInSection(t *testing.T) {
	got := patchIni(sampleIni, []IniOverride{
		{Section: "TRANSPORT", Key: "extra_key", Value: "1"},
	})
	lines := activeLines(got)
	if !lines["TRANSPORT/extra_key = 1"] {
		t.Fatalf("missing key not inserted:\n%s", got)
	}
	// inserted directly after the [TRANSPORT] header
	idx := strings.Index(got, "[TRANSPORT]\nextra_key = 1")
	if idx < 0 {
		t.Fatalf("key not inserted after section header:\n%s", got)
	}
}

func TestPatchIniAppendsMissingSection(t *testing.T) {
	got := patchIni(sampleIni, []IniOverride{
		{Section: "PROXY", Key: "socks_proxy", Value: "localhost:9050"},
	})
	if !strings.Contains(got, "[PROXY]\nsocks_proxy = localhost:9050") {
		t.Fatalf("missing section not appended:\n%s", got)
	}
}

func TestPatchIniLeavesOtherSectionsAlone(t *testing.T) {
	got := patchIni(sampleIni, []IniOverride{
		{Section: "STORE_LOG", Key: "enable", Value: "on"},
	})
	lines := activeLines(got)
	if !lines["STORE_LOG/enable = on"] {
		t.Fatalf("STORE_LOG enable not set")
	}
	// [INACTIVE_CLIENTS] disconnect = on must be untouched, and WEB comments preserved
	if !lines["INACTIVE_CLIENTS/disconnect = on"] {
		t.Fatalf("unrelated section modified")
	}
	if !strings.Contains(got, "# https = 443") {
		t.Fatalf("unrelated comments modified")
	}
}

func TestBuildIniOverridesFullRewrite(t *testing.T) {
	cfg := Config{Port: 5233, StoreLog: true, DailyStats: true, QueuePassword: "hunter22!"}
	ovs, err := buildIniOverrides(cfg, "172.18.0.5")
	if err != nil {
		t.Fatal(err)
	}
	got := patchIni(sampleIni, ovs)
	lines := activeLines(got)
	for _, want := range []string{
		"TRANSPORT/host = 172.18.0.5",
		"TRANSPORT/port = 5233",
		"TRANSPORT/websockets = off",
		"STORE_LOG/enable = on",
		"STORE_LOG/restore_messages = on",
		"STORE_LOG/log_stats = on",
		"AUTH/new_queues = on",
		"AUTH/create_password = hunter22!",
	} {
		if !lines[want] {
			t.Errorf("missing expected line %q\n%s", want, got)
		}
	}
}

func TestBuildIniOverridesNoPassword(t *testing.T) {
	cfg := Config{Port: 5223}
	ovs, err := buildIniOverrides(cfg, "172.18.0.5")
	if err != nil {
		t.Fatal(err)
	}
	got := patchIni(sampleIni, ovs)
	lines := activeLines(got)
	for k := range lines {
		if strings.HasPrefix(k, "AUTH/create_password") {
			t.Fatalf("create_password must not be active without a password: %s", k)
		}
	}
}

func TestBuildIniOverridesRejectsBadInput(t *testing.T) {
	if _, err := buildIniOverrides(Config{Port: 0}, "172.18.0.5"); err == nil {
		t.Error("port 0 accepted")
	}
	if _, err := buildIniOverrides(Config{Port: 5223}, ""); err == nil {
		t.Error("empty IP accepted")
	}
	if _, err := buildIniOverrides(Config{Port: 5223}, "bad ip\n[X]"); err == nil {
		t.Error("IP with ini metacharacters accepted")
	}
	if _, err := buildIniOverrides(Config{Port: 5223, QueuePassword: "has space"}, "172.18.0.5"); err == nil {
		t.Error("invalid stored password accepted")
	}
}

func TestBuildAddress(t *testing.T) {
	fp := "d5fcsc7hhtPpexYUbI2XPxDbyU2d3WsVmROimcL90ss="
	if got := buildAddress(fp, "172.18.0.5", 5223); got != "smp://"+fp+"@172.18.0.5" {
		t.Errorf("default port must be omitted, got %q", got)
	}
	if got := buildAddress(fp, "172.18.0.5", 443); got != "smp://"+fp+"@172.18.0.5:443" {
		t.Errorf("non-default port must be included, got %q", got)
	}
	if got := buildAddress("", "172.18.0.5", 5223); got != "" {
		t.Errorf("no fingerprint must yield empty address, got %q", got)
	}
	if got := buildAddress(fp, "", 5223); got != "" {
		t.Errorf("no host must yield empty address, got %q", got)
	}
}
