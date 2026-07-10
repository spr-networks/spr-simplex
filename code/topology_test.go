package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// The SPR host merges the plugin graph into the router topology at the root
// anchor node — its shape is a contract (mirrors spr-tailscale).
func TestTopologyRootAnchor(t *testing.T) {
	topo := buildTopology(true, "172.18.0.5")
	if len(topo.Nodes) == 0 {
		t.Fatal("no nodes")
	}
	root := topo.Nodes[0]
	if root.ID != "root" || root.ConnType != "smp" || !root.Online {
		t.Errorf("bad root anchor: %+v", root)
	}
	if root.Kind != "" || root.Name != "" || root.IP != "" {
		t.Errorf("root anchor must carry only ID/ConnType/Online: %+v", root)
	}
}

func TestTopologyServiceNode(t *testing.T) {
	topo := buildTopology(true, "172.18.0.5")
	if len(topo.Nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(topo.Nodes))
	}
	svc := topo.Nodes[1]
	if svc.ID != "smp-server" || svc.Kind != "service" || svc.ConnType != "smp" {
		t.Errorf("bad service node: %+v", svc)
	}
	if !svc.Online || svc.IP != "172.18.0.5" {
		t.Errorf("service node must reflect live state: %+v", svc)
	}
	if len(topo.Edges) != 1 {
		t.Fatalf("want 1 edge, got %d", len(topo.Edges))
	}
	e := topo.Edges[0]
	if e.From != "root" || e.To != "smp-server" || e.Layer != "l1" || e.Kind != "smp" {
		t.Errorf("bad edge: %+v", e)
	}
}

func TestTopologyDaemonDown(t *testing.T) {
	topo := buildTopology(false, "")
	if topo.Nodes[0].Online != true {
		t.Error("root anchor is always online")
	}
	if topo.Nodes[1].Online {
		t.Error("service node must be offline when the daemon is down")
	}
	// IP is omitempty: unknown IP must not serialize
	data, err := json.Marshal(topo)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"IP"`) {
		t.Errorf("empty IP must be omitted: %s", data)
	}
	if !strings.Contains(string(data), `"Nodes"`) || !strings.Contains(string(data), `"Edges"`) {
		t.Errorf("missing contract keys: %s", data)
	}
}
