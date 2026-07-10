package main

import (
	"fmt"
	"strconv"
	"strings"
)

// smp-server generates /etc/opt/simplex/smp-server.ini once, at `smp-server
// init`. The plugin owns a small set of keys in that file and rewrites them
// from the plugin config before every daemon start; everything else
// (comments, defaults, unmanaged keys) is preserved verbatim.

// IniOverride sets (or deletes) one `key = value` line inside [Section].
type IniOverride struct {
	Section string
	Key     string
	Value   string
	Delete  bool
}

// patchIni applies overrides to ini file content. For each override it
// replaces the first matching `key = ...` line in the section — active or
// commented out (`# key = ...`) — with an active line. Missing keys are
// inserted right after the section header; a missing section is appended at
// the end. Delete removes the key's line entirely.
func patchIni(content string, overrides []IniOverride) string {
	lines := strings.Split(content, "\n")

	for _, ov := range overrides {
		lines = patchOne(lines, ov)
	}
	return strings.Join(lines, "\n")
}

func patchOne(lines []string, ov IniOverride) []string {
	section := ""
	sectionHeaderIdx := -1
	newLine := ov.Key + " = " + ov.Value

	for i, line := range lines {
		if s, ok := parseSectionHeader(line); ok {
			section = s
			if s == ov.Section && sectionHeaderIdx == -1 {
				sectionHeaderIdx = i
			}
			continue
		}
		if section != ov.Section {
			continue
		}
		if !lineHasKey(line, ov.Key) {
			continue
		}
		if ov.Delete {
			return append(lines[:i], lines[i+1:]...)
		}
		lines[i] = newLine
		return lines
	}

	if ov.Delete {
		return lines
	}
	if sectionHeaderIdx >= 0 {
		// insert right after the section header
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:sectionHeaderIdx+1]...)
		out = append(out, newLine)
		out = append(out, lines[sectionHeaderIdx+1:]...)
		return out
	}
	// section missing entirely: append it
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	return append(lines, "["+ov.Section+"]", newLine)
}

func parseSectionHeader(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if len(t) >= 2 && t[0] == '[' && t[len(t)-1] == ']' {
		return t[1 : len(t)-1], true
	}
	return "", false
}

// lineHasKey reports whether line defines key (`key = ...`), possibly
// commented out (`# key = ...`). Prose comments that merely mention the key
// ("# Use create_password option to ...") do not match: after stripping one
// comment marker the line must start with the key followed by '='.
func lineHasKey(line, key string) bool {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "#") {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
	}
	if !strings.HasPrefix(t, key) {
		return false
	}
	rest := strings.TrimSpace(t[len(key):])
	return strings.HasPrefix(rest, "=")
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// buildIniOverrides maps the plugin config to the smp-server.ini keys the
// plugin manages. host/ip only affects the address smp-server prints; the
// listener binds per [TRANSPORT] port on all container interfaces (the
// container only has the spr-simplex bridge interface + loopback).
func buildIniOverrides(cfg Config, ip string) ([]IniOverride, error) {
	if err := validatePort(cfg.Port); err != nil {
		return nil, err
	}
	if cfg.QueuePassword != "" {
		if err := validateQueuePassword(cfg.QueuePassword); err != nil {
			return nil, err
		}
	}
	if strings.ContainsAny(ip, " \t\n#[]=") || ip == "" {
		return nil, fmt.Errorf("invalid container IP %q", ip)
	}
	ovs := []IniOverride{
		{Section: "TRANSPORT", Key: "host", Value: ip},
		{Section: "TRANSPORT", Key: "port", Value: strconv.Itoa(cfg.Port)},
		{Section: "TRANSPORT", Key: "websockets", Value: "off"},
		{Section: "STORE_LOG", Key: "enable", Value: onOff(cfg.StoreLog)},
		{Section: "STORE_LOG", Key: "restore_messages", Value: onOff(cfg.StoreLog)},
		{Section: "STORE_LOG", Key: "log_stats", Value: onOff(cfg.DailyStats)},
		{Section: "AUTH", Key: "new_queues", Value: "on"},
	}
	if cfg.QueuePassword != "" {
		ovs = append(ovs, IniOverride{Section: "AUTH", Key: "create_password", Value: cfg.QueuePassword})
	} else {
		ovs = append(ovs, IniOverride{Section: "AUTH", Key: "create_password", Delete: true})
	}
	return ovs, nil
}

// buildAddress renders the copyable SMP server address. Port 5223 is the
// protocol default and is omitted, matching how smp-server itself prints it.
func buildAddress(fingerprint, host string, port int) string {
	if fingerprint == "" || host == "" {
		return ""
	}
	addr := "smp://" + fingerprint + "@" + host
	if port != DefaultSMPPort {
		addr += ":" + strconv.Itoa(port)
	}
	return addr
}
