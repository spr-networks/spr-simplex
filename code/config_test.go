package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePort(t *testing.T) {
	for _, p := range []int{1, 443, 5223, 65535} {
		if err := validatePort(p); err != nil {
			t.Errorf("port %d rejected: %v", p, err)
		}
	}
	for _, p := range []int{0, -1, 65536, 100000} {
		if err := validatePort(p); err == nil {
			t.Errorf("port %d accepted", p)
		}
	}
}

func TestValidateQueuePassword(t *testing.T) {
	valid := []string{"hunter22!", "abcdefgh", strings.Repeat("x", 128), "P4ss-word_%^&*"}
	for _, pw := range valid {
		if err := validateQueuePassword(pw); err != nil {
			t.Errorf("%q rejected: %v", pw, err)
		}
	}
	invalid := []string{
		"",                       // empty
		"short7!",                // < 8 chars
		strings.Repeat("x", 129), // > 128 chars
		"has space",              // whitespace
		"has\ttab8",              // whitespace
		"with@sign",              // '@' breaks smp:// address embedding
		"with:colon",             // ':'
		"with/slash",             // '/'
		"pässword",               // non-ASCII
		"newline\npw8",           // control char
	}
	for _, pw := range invalid {
		if err := validateQueuePassword(pw); err == nil {
			t.Errorf("%q accepted", pw)
		}
	}
}

func TestApplyUpdateKeepsPasswordWhenEmpty(t *testing.T) {
	cur := Config{Port: 5223, QueuePassword: "keepMe88"}
	next, err := applyUpdate(cur, ConfigUpdate{Port: 5224, StoreLog: true})
	if err != nil {
		t.Fatal(err)
	}
	if next.QueuePassword != "keepMe88" {
		t.Errorf("empty QueuePassword must keep the stored one, got %q", next.QueuePassword)
	}
	if next.Port != 5224 || !next.StoreLog {
		t.Errorf("other fields not applied: %+v", next)
	}
}

func TestApplyUpdateReplacesPassword(t *testing.T) {
	cur := Config{Port: 5223, QueuePassword: "oldpass8"}
	next, err := applyUpdate(cur, ConfigUpdate{Port: 5223, QueuePassword: "newpass8"})
	if err != nil {
		t.Fatal(err)
	}
	if next.QueuePassword != "newpass8" {
		t.Errorf("password not replaced, got %q", next.QueuePassword)
	}
}

func TestApplyUpdateClearsPassword(t *testing.T) {
	cur := Config{Port: 5223, QueuePassword: "oldpass8"}
	next, err := applyUpdate(cur, ConfigUpdate{Port: 5223, ClearQueuePassword: true})
	if err != nil {
		t.Fatal(err)
	}
	if next.QueuePassword != "" {
		t.Errorf("password not cleared, got %q", next.QueuePassword)
	}
}

func TestApplyUpdateRejectsInvalid(t *testing.T) {
	cur := defaultConfig()
	if _, err := applyUpdate(cur, ConfigUpdate{Port: 0}); err == nil {
		t.Error("invalid port accepted")
	}
	if _, err := applyUpdate(cur, ConfigUpdate{Port: 5223, QueuePassword: "no@good8"}); err == nil {
		t.Error("invalid password accepted")
	}
	// a failed update must not mutate the current config
	if _, err := applyUpdate(cur, ConfigUpdate{Port: 5223, QueuePassword: "bad pw 8"}); err == nil {
		t.Error("invalid password accepted")
	}
}

func TestRedactConfigNeverEchoesPassword(t *testing.T) {
	resp := redactConfig(Config{Port: 5223, QueuePassword: "superSecret9"})
	if !resp.QueuePasswordSet {
		t.Error("QueuePasswordSet should be true")
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "superSecret9") {
		t.Errorf("secret leaked in GET /config response: %s", data)
	}
	if redactConfig(Config{Port: 5223}).QueuePasswordSet {
		t.Error("QueuePasswordSet should be false without a password")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	orig := ConfigFile
	ConfigFile = filepath.Join(dir, "config.json")
	defer func() { ConfigFile = orig }()

	Configmtx.Lock()
	gConfig = Config{Port: 5224, StoreLog: true, DailyStats: false, QueuePassword: "roundtrip8"}
	err := writeConfigLocked()
	Configmtx.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("config file mode = %o, want 0600", info.Mode().Perm())
	}

	Configmtx.Lock()
	gConfig = defaultConfig()
	Configmtx.Unlock()
	if err := loadConfig(); err != nil {
		t.Fatal(err)
	}
	got := currentConfig()
	if got.Port != 5224 || !got.StoreLog || got.DailyStats || got.QueuePassword != "roundtrip8" {
		t.Errorf("round trip mismatch: %+v", got)
	}
}

func TestLoadConfigFirstBootWritesDefaults(t *testing.T) {
	dir := t.TempDir()
	orig := ConfigFile
	ConfigFile = filepath.Join(dir, "config.json")
	defer func() { ConfigFile = orig }()

	Configmtx.Lock()
	gConfig = defaultConfig()
	Configmtx.Unlock()
	if err := loadConfig(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ConfigFile); err != nil {
		t.Errorf("defaults not persisted on first boot: %v", err)
	}
	if got := currentConfig(); got.Port != DefaultSMPPort {
		t.Errorf("default port = %d, want %d", got.Port, DefaultSMPPort)
	}
}
