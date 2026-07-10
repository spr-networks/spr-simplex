package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var UNIX_PLUGIN_LISTENER = "/state/plugins/spr-simplex/socket"

// Status is the GET /status response.
type Status struct {
	Running          bool
	InitDone         bool
	StartedAt        string // RFC3339; "" when not running
	UptimeSeconds    int64
	Version          string
	Fingerprint      string
	Address          string
	Host             string
	Port             int
	QueuePasswordSet bool
	StoreLog         bool
	DailyStats       bool
}

// Address is the GET /address response — the copyable SMP server address.
type Address struct {
	Address          string
	Fingerprint      string
	Host             string
	Port             int
	QueuePasswordSet bool
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Println("encoding response failed:", err)
	}
}

func currentStatus() Status {
	cfg := currentConfig()
	fp := Fingerprint()
	host := getContainerIP()
	running := gDaemon.Running()

	st := Status{
		Running:          running,
		InitDone:         fp != "",
		Version:          gDaemon.Version(),
		Fingerprint:      fp,
		Address:          buildAddress(fp, host, cfg.Port),
		Host:             host,
		Port:             cfg.Port,
		QueuePasswordSet: cfg.QueuePassword != "",
		StoreLog:         cfg.StoreLog,
		DailyStats:       cfg.DailyStats,
	}
	if started := gDaemon.StartedAt(); running && !started.IsZero() {
		st.StartedAt = started.UTC().Format(time.RFC3339)
		st.UptimeSeconds = int64(time.Since(started).Seconds())
	}
	return st
}

func handleGetStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, currentStatus())
}

func handleGetAddress(w http.ResponseWriter, r *http.Request) {
	cfg := currentConfig()
	fp := Fingerprint()
	host := getContainerIP()
	writeJSON(w, Address{
		Address:          buildAddress(fp, host, cfg.Port),
		Fingerprint:      fp,
		Host:             host,
		Port:             cfg.Port,
		QueuePasswordSet: cfg.QueuePassword != "",
	})
}

func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, redactConfig(currentConfig()))
}

func handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var upd ConfigUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), 400)
		return
	}

	Configmtx.Lock()
	next, err := applyUpdate(gConfig, upd)
	if err != nil {
		Configmtx.Unlock()
		http.Error(w, err.Error(), 400)
		return
	}
	gConfig = next
	err = writeConfigLocked()
	Configmtx.Unlock()
	if err != nil {
		http.Error(w, "persisting config failed: "+err.Error(), 500)
		return
	}

	// apply: rewrite smp-server.ini and restart the daemon
	if err := gDaemon.Restart(); err != nil {
		http.Error(w, "config saved but restart failed: "+err.Error(), 500)
		return
	}
	writeJSON(w, redactConfig(next))
}

func handleRestart(w http.ResponseWriter, r *http.Request) {
	if err := gDaemon.Restart(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, currentStatus())
}

func handleGetTopology(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, buildTopology(gDaemon.Running(), getContainerIP()))
}

type spaHandler struct {
	staticPath string
	indexPath  string
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path, err := filepath.Abs(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path = filepath.Join(h.staticPath, path)
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

func logRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

func main() {
	if err := loadConfig(); err != nil {
		log.Println("loading config failed (continuing with defaults):", err)
	}

	if err := gDaemon.Start(); err != nil {
		// keep the API up so the UI can show the error and retry via /restart
		log.Println("starting smp-server failed:", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", handleGetStatus)
	mux.HandleFunc("GET /address", handleGetAddress)
	mux.HandleFunc("GET /config", handleGetConfig)
	mux.HandleFunc("PUT /config", handlePutConfig)
	mux.HandleFunc("POST /restart", handleRestart)
	mux.HandleFunc("GET /topology", handleGetTopology)

	// serve the bundled UI for everything else
	mux.Handle("/", spaHandler{staticPath: "/ui", indexPath: "index.html"})

	os.Remove(UNIX_PLUGIN_LISTENER)
	if err := os.MkdirAll(filepath.Dir(UNIX_PLUGIN_LISTENER), 0755); err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("unix", UNIX_PLUGIN_LISTENER)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.Chmod(UNIX_PLUGIN_LISTENER, 0770); err != nil {
		log.Fatal(err)
	}

	// terminate smp-server cleanly when the container stops
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		gDaemon.Stop()
		listener.Close()
		os.Exit(0)
	}()

	server := http.Server{Handler: logRequest(mux)}
	if err := server.Serve(listener); err != nil {
		log.Fatal(err)
	}
}
