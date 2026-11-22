package gatus

import "testing"

func results(successes ...bool) []Result {
	out := make([]Result, len(successes))
	for i, s := range successes {
		out[i] = Result{Success: s}
	}
	return out
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		want    string
	}{
		{"no results", nil, StateUnknown},
		{"all success", results(true, true, true), StateUp},
		{"last failed", results(true, true, false), StateDown},
		{"recovered", results(false, true, true), StateFlapping},
		{"single failure", results(false), StateDown},
	}
	for _, tt := range tests {
		if got := Classify(tt.results); got != tt.want {
			t.Errorf("%s: Classify() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestLabel(t *testing.T) {
	tests := map[string]string{
		"Proxmox pve04 - tcp://pve04.iac.lan:22":                  "Proxmox pve04",
		"Shelly Office Light- icmp://shelly-office-light.iac.lan": "Shelly Office Light",
		"FW-GUI Telemetry - https://telemetry.fw-gui.com":         "FW-GUI Telemetry",
		"Anycast DNS 1.1.1.1":                                     "Anycast DNS 1.1.1.1",
		"  - leading separator":                                   "- leading separator",
	}
	for in, want := range tests {
		if got := Label(in); got != want {
			t.Errorf("Label(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildGroups(t *testing.T) {
	groups := BuildGroups([]EndpointStatus{
		{Name: "b", Group: "Web", Key: "web_b", Results: results(true)},
		{Name: "a", Group: "Web", Key: "web_a", Results: results(false)},
		{Name: "x", Key: "_x"},
		{Name: "dns", Group: "DNS", Key: "dns_dns", Results: results(false, false, true)},
	}, 2)
	wantOrder := []string{"DNS", ungroupedName, "Web"}
	if len(groups) != len(wantOrder) {
		t.Fatalf("got %d groups, want %d", len(groups), len(wantOrder))
	}
	for i, name := range wantOrder {
		if groups[i].Name != name {
			t.Errorf("group %d = %q, want %q", i, groups[i].Name, name)
		}
	}
	if dns := groups[0].Endpoints[0]; dns.State != StateFlapping || len(dns.Recent) != 2 {
		t.Errorf("history not trimmed to 2: %+v", dns)
	}
	web := groups[2].Endpoints
	if web[0].Key != "web_a" || web[0].State != StateDown || web[1].State != StateUp {
		t.Errorf("unexpected web pills: %+v", web)
	}
}
