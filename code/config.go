package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var TEST_PREFIX = os.Getenv("TEST_PREFIX")

var (
	ConfigFile      = TEST_PREFIX + "/configs/spr-simplex/config.json"
	SMPConfDir      = TEST_PREFIX + "/etc/opt/simplex"
	SMPLogDir       = TEST_PREFIX + "/var/opt/simplex"
	FingerprintFile = TEST_PREFIX + "/etc/opt/simplex/fingerprint"
	IniFile         = TEST_PREFIX + "/etc/opt/simplex/smp-server.ini"
)

// DefaultSMPPort is the SMP protocol default; addresses omit it
// (smp://fingerprint@host implies :5223).
const DefaultSMPPort = 5223

// Config is the plugin configuration persisted at
// /configs/spr-simplex/config.json (mode 0600 — QueuePassword is a secret and
// is never echoed by the API).
type Config struct {
	// Port smp-server listens on, bound to the container IP on the
	// spr-simplex bridge.
	Port int
	// StoreLog enables smp-server's append-only store log so queues and
	// undelivered messages survive restarts. Off by default (upstream default).
	StoreLog bool
	// DailyStats enables daily aggregate statistics logging (CSV in
	// /var/opt/simplex). Off by default.
	DailyStats bool
	// QueuePassword, when set, is required to create new messaging queues
	// (SMP basic auth). Clients include it in the server address:
	// smp://fingerprint:password@host. Empty = anyone who can reach the
	// relay may create queues.
	QueuePassword string
}

var (
	Configmtx sync.RWMutex
	gConfig   = defaultConfig()
)

func defaultConfig() Config {
	return Config{Port: DefaultSMPPort}
}

// ConfigResponse is what GET /config returns: the queue password is redacted
// to a boolean.
type ConfigResponse struct {
	Port             int
	StoreLog         bool
	DailyStats       bool
	QueuePasswordSet bool
}

func redactConfig(c Config) ConfigResponse {
	return ConfigResponse{
		Port:             c.Port,
		StoreLog:         c.StoreLog,
		DailyStats:       c.DailyStats,
		QueuePasswordSet: c.QueuePassword != "",
	}
}

// ConfigUpdate is the PUT /config body. QueuePassword semantics: empty keeps
// the stored password, non-empty replaces it, ClearQueuePassword removes it.
type ConfigUpdate struct {
	Port               int
	StoreLog           bool
	DailyStats         bool
	QueuePassword      string
	ClearQueuePassword bool
}

func validatePort(p int) error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

// validateQueuePassword enforces smp-server's charset for create_password:
// printable ASCII without whitespace, '@', ':' and '/' (the password is
// embedded in smp:// addresses and in smp-server.ini).
func validateQueuePassword(pw string) error {
	if len(pw) < 8 || len(pw) > 128 {
		return fmt.Errorf("queue password must be 8-128 characters")
	}
	for _, r := range pw {
		if r <= ' ' || r > '~' || strings.ContainsRune("@:/", r) {
			return fmt.Errorf("queue password may only contain printable ASCII characters, excluding spaces, '@', ':' and '/'")
		}
	}
	return nil
}

// applyUpdate validates upd against cur and returns the resulting config.
func applyUpdate(cur Config, upd ConfigUpdate) (Config, error) {
	if err := validatePort(upd.Port); err != nil {
		return cur, err
	}
	next := Config{
		Port:          upd.Port,
		StoreLog:      upd.StoreLog,
		DailyStats:    upd.DailyStats,
		QueuePassword: cur.QueuePassword,
	}
	if upd.ClearQueuePassword {
		next.QueuePassword = ""
	} else if upd.QueuePassword != "" {
		if err := validateQueuePassword(upd.QueuePassword); err != nil {
			return cur, err
		}
		next.QueuePassword = upd.QueuePassword
	}
	return next, nil
}

func loadConfig() error {
	Configmtx.Lock()
	defer Configmtx.Unlock()
	data, err := os.ReadFile(ConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			// first boot: persist the defaults
			return writeConfigLocked()
		}
		return err
	}
	cfg := defaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	if err := validatePort(cfg.Port); err != nil {
		return err
	}
	if cfg.QueuePassword != "" {
		if err := validateQueuePassword(cfg.QueuePassword); err != nil {
			return err
		}
	}
	gConfig = cfg
	return nil
}

// writeConfigLocked atomically persists gConfig (callers hold Configmtx).
func writeConfigLocked() error {
	data, err := json.MarshalIndent(gConfig, "", " ")
	if err != nil {
		return err
	}
	return atomicWrite(ConfigFile, data, 0600)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func currentConfig() Config {
	Configmtx.RLock()
	defer Configmtx.RUnlock()
	return gConfig
}
