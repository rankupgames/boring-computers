package main

import "testing"

func TestEffectiveNetworkEnabledRequiresHostAndRequestApproval(t *testing.T) {
	for _, test := range []struct {
		name      string
		host      bool
		requested bool
		want      bool
	}{
		{name: "both enabled", host: true, requested: true, want: true},
		{name: "request denied", host: true, requested: false, want: false},
		{name: "host denied", host: false, requested: true, want: false},
		{name: "both denied", host: false, requested: false, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := Config{NetEnable: test.host}
			if got := effectiveNetworkEnabled(cfg, test.requested); got != test.want {
				t.Fatalf("effectiveNetworkEnabled() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestTemplateSnapshotMustMatchRequestedNetwork(t *testing.T) {
	withoutNIC := Template{Snapshot: true}
	withNIC := Template{Snapshot: true, RestoreNet: true}
	notSnapshot := Template{}

	for _, test := range []struct {
		name    string
		tpl     Template
		network bool
		want    bool
	}{
		{name: "offline snapshot for offline request", tpl: withoutNIC, want: true},
		{name: "offline snapshot for online request", tpl: withoutNIC, network: true, want: false},
		{name: "online snapshot for online request", tpl: withNIC, network: true, want: true},
		{name: "online snapshot for offline request", tpl: withNIC, want: false},
		{name: "non snapshot template", tpl: notSnapshot, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := templateSnapshotMatchesNetwork(test.tpl, test.network); got != test.want {
				t.Fatalf("templateSnapshotMatchesNetwork() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestWarmPoolDoesNotCrossRequestNetworkBoundary(t *testing.T) {
	pooled := &Machine{driver: &fcDriver{netEnabled: true}}
	mgr := &Manager{pool: []*Machine{pooled}}

	if got := mgr.claimPooled("127.0.0.1", 60, false, false); got != nil {
		t.Fatal("claimPooled() returned a networked machine for an offline request")
	}
	if len(mgr.pool) != 1 || mgr.pool[0] != pooled {
		t.Fatal("claimPooled() removed a machine after rejecting its network state")
	}
}

func TestWarmPoolClaimsMatchingOfflineMachine(t *testing.T) {
	pooled := &Machine{driver: &fcDriver{netEnabled: false}}
	mgr := &Manager{pool: []*Machine{pooled}}

	if got := mgr.claimPooled("127.0.0.1", 60, false, false); got != pooled {
		t.Fatal("claimPooled() did not return the matching offline machine")
	}
	if len(mgr.pool) != 0 {
		t.Fatal("claimPooled() left a claimed machine in the pool")
	}
}
