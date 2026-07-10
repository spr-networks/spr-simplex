package main

// Topology contribution merged into SPR's router topology view. The struct
// shapes and the root anchor node mirror the spr-tailscale contract: the SPR
// host attaches the plugin graph to the router topology at the "root" node.

type TopoNode struct {
	ID       string
	Kind     string
	Name     string
	IP       string `json:",omitempty"`
	ConnType string `json:",omitempty"`
	Online   bool
}

type TopoEdge struct {
	From  string
	To    string
	Layer string
	Kind  string
}

type Topology struct {
	Nodes []TopoNode
	Edges []TopoEdge
}

// buildTopology emits the root anchor plus one service node for the SMP
// relay. Online reflects the live daemon state; IP is the container IP on the
// spr-simplex bridge (omitted when unknown).
func buildTopology(running bool, ip string) Topology {
	topo := Topology{
		Nodes: []TopoNode{{ID: "root", ConnType: "smp", Online: true}},
		Edges: []TopoEdge{},
	}
	topo.Nodes = append(topo.Nodes, TopoNode{
		ID:       "smp-server",
		Kind:     "service",
		Name:     "SMP relay",
		IP:       ip,
		ConnType: "smp",
		Online:   running,
	})
	topo.Edges = append(topo.Edges, TopoEdge{From: "root", To: "smp-server", Layer: "l1", Kind: "smp"})
	return topo
}
