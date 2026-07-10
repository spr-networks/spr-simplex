package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var SMPServerBin = "smp-server"

// PinnedSMPVersion is stamped by the Dockerfile via
// -ldflags "-X main.PinnedSMPVersion=v6.5.0" (fallback when `smp-server -v`
// cannot be executed).
var PinnedSMPVersion = ""

// Daemon supervises the smp-server child process: first-start initialization
// (keys/cert/fingerprint), ini rewrite from the plugin config, crash restart.
type Daemon struct {
	mtx        sync.Mutex
	cmd        *exec.Cmd
	generation int
	stopped    bool
	startedAt  time.Time
	version    string
}

var gDaemon = &Daemon{}

// getContainerIP returns the container's IPv4 address on eth0
// (the spr-simplex docker bridge).
func getContainerIP() string {
	iface, err := net.InterfaceByName("eth0")
	if err != nil {
		return ""
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return ""
}

func listenIP() string {
	if ip := getContainerIP(); ip != "" {
		return ip
	}
	// dev / test fallback
	return "127.0.0.1"
}

// Initialized reports whether `smp-server init` has run: the certificate
// fingerprint (= the server identity) exists. It lives on a bind mount under
// /state/plugins/spr-simplex so it survives container rebuilds.
func Initialized() bool {
	_, err := os.Stat(FingerprintFile)
	return err == nil
}

// Fingerprint returns the persisted certificate fingerprint (base64), or "".
func Fingerprint() string {
	data, err := os.ReadFile(FingerprintFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ensureInit runs `smp-server init` once per server identity. init generates
// the offline CA + online TLS certificate and the fingerprint under
// /etc/opt/simplex. It must never run again while a fingerprint exists —
// upstream init wipes the config dir, which would change the server identity
// and strand every client.
func ensureInit(ip string) error {
	if Initialized() {
		return nil
	}
	if err := os.MkdirAll(SMPConfDir, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(SMPLogDir, 0700); err != nil {
		return err
	}
	log.Printf("first start: initializing smp-server identity (ip %s)", ip)
	// --no-password: queue-creation auth is managed via the plugin config
	// ([AUTH] create_password rewrite), not at init time.
	// --source-code: records the upstream repo in [INFORMATION] (AGPLv3).
	// --disable-web: no embedded web server; the ini rewrite keeps every
	// listener off except the SMP port.
	cmd := exec.Command(SMPServerBin, "init", "-y",
		"--ip", ip, "--no-password", "--source-code", "--disable-web")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("smp-server init failed: %v", err)
	}
	tightenPerms()
	if !Initialized() {
		return fmt.Errorf("smp-server init did not produce a fingerprint")
	}
	return nil
}

// tightenPerms enforces 0700 on the smp-server state dirs and 0600 on the
// files in the config dir (CA key, TLS key/cert, fingerprint, ini).
func tightenPerms() {
	for _, dir := range []string{SMPConfDir, SMPLogDir} {
		os.Chmod(dir, 0700)
	}
	entries, err := os.ReadDir(SMPConfDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Type().IsRegular() {
			os.Chmod(filepath.Join(SMPConfDir, e.Name()), 0600)
		}
	}
}

// applyIni rewrites the plugin-managed keys in smp-server.ini from cfg.
func applyIni(cfg Config, ip string) error {
	ovs, err := buildIniOverrides(cfg, ip)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(IniFile)
	if err != nil {
		return fmt.Errorf("reading %s: %v", IniFile, err)
	}
	patched := patchIni(string(data), ovs)
	return atomicWrite(IniFile, []byte(patched), 0600)
}

// Start initializes (first start only), applies the config to smp-server.ini
// and launches `smp-server start`, restarting it with a delay if it dies
// unexpectedly.
func (d *Daemon) Start() error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return d.startLocked()
}

func (d *Daemon) startLocked() error {
	cfg := currentConfig()
	ip := listenIP()

	if err := ensureInit(ip); err != nil {
		return err
	}
	if err := applyIni(cfg, ip); err != nil {
		return err
	}
	tightenPerms()

	cmd := exec.Command(SMPServerBin, "start")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting smp-server: %v", err)
	}
	d.cmd = cmd
	d.stopped = false
	d.startedAt = time.Now()
	d.generation++
	gen := d.generation
	log.Printf("smp-server started (pid %d, port %d)", cmd.Process.Pid, cfg.Port)

	go func() {
		err := cmd.Wait()
		d.mtx.Lock()
		if d.generation != gen || d.stopped {
			d.mtx.Unlock()
			return
		}
		d.cmd = nil
		d.mtx.Unlock()
		log.Printf("smp-server exited unexpectedly: %v; restarting in 5s", err)
		time.Sleep(5 * time.Second)
		d.mtx.Lock()
		defer d.mtx.Unlock()
		if d.generation == gen && !d.stopped {
			if err := d.startLocked(); err != nil {
				log.Printf("smp-server restart failed: %v", err)
			}
		}
	}()
	return nil
}

func (d *Daemon) stopLocked() {
	d.stopped = true
	d.generation++
	if d.cmd != nil && d.cmd.Process != nil {
		proc := d.cmd.Process
		proc.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			d.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			proc.Kill()
		}
	}
	d.cmd = nil
}

// Restart stops smp-server (if running), reapplies the ini and starts it again.
func (d *Daemon) Restart() error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.stopLocked()
	return d.startLocked()
}

// Stop terminates smp-server for plugin shutdown.
func (d *Daemon) Stop() {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.stopLocked()
}

// Running reports whether the smp-server child process is alive.
func (d *Daemon) Running() bool {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return d.cmd != nil && d.cmd.Process != nil && d.cmd.ProcessState == nil
}

// StartedAt returns the current process start time (zero when not running).
func (d *Daemon) StartedAt() time.Time {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	if d.cmd == nil {
		return time.Time{}
	}
	return d.startedAt
}

// Version returns the smp-server version banner, captured once from
// `smp-server -v`, falling back to the version pinned at build time.
func (d *Daemon) Version() string {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	if d.version == "" {
		out, err := exec.Command(SMPServerBin, "-v").Output()
		if err == nil {
			if line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]); line != "" {
				d.version = line
			}
		}
		if d.version == "" {
			d.version = PinnedSMPVersion
		}
	}
	return d.version
}
