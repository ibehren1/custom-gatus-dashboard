package gatus

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// Endpoint states shown as pill colors.
const (
	StateUp       = "up"
	StateFlapping = "flapping"
	StateDown     = "down"
	StateUnknown  = "unknown"
)

const ungroupedName = "Ungrouped"

// EndpointStatus mirrors an element of Gatus' /api/v1/endpoints/statuses response.
type EndpointStatus struct {
	Name    string   `json:"name"`
	Group   string   `json:"group"`
	Key     string   `json:"key"`
	Results []Result `json:"results"`
}

// Result mirrors a single check result. Gatus returns results oldest first.
type Result struct {
	Hostname  string        `json:"hostname"`
	Duration  time.Duration `json:"duration"`
	Errors    []string      `json:"errors"`
	Success   bool          `json:"success"`
	Timestamp time.Time     `json:"timestamp"`
}

// Summary is the payload served to the dashboard UI.
type Summary struct {
	Title          string    `json:"title"`
	Version        string    `json:"version"`
	RefreshSeconds int       `json:"refreshSeconds"`
	Updated        time.Time `json:"updated"`
	Groups         []Group   `json:"groups"`
}

type Group struct {
	Name      string `json:"name"`
	Endpoints []Pill `json:"endpoints"`
}

type Pill struct {
	Key           string    `json:"key"`
	Label         string    `json:"label"`
	Name          string    `json:"name"`
	State         string    `json:"state"`
	LastDuration  string    `json:"lastDuration,omitempty"`
	LastTimestamp time.Time `json:"lastTimestamp,omitzero"`
	Errors        []string  `json:"errors,omitempty"`
	Recent        []bool    `json:"recent"`
}

// Classify derives the pill state from results ordered oldest first.
func Classify(results []Result) string {
	if len(results) == 0 {
		return StateUnknown
	}
	if !results[len(results)-1].Success {
		return StateDown
	}
	for _, r := range results {
		if !r.Success {
			return StateFlapping
		}
	}
	return StateUp
}

// labelSeparator matches the first dash followed by whitespace, so
// "Proxmox pve04 - tcp://..." and "Shelly Office Light- icmp://..." both split,
// while hyphenated words like "FW-GUI" do not.
var labelSeparator = regexp.MustCompile(`\s*-\s+`)

// Label shortens an endpoint name for display on a pill.
func Label(name string) string {
	label := strings.TrimSpace(labelSeparator.Split(name, 2)[0])
	if label == "" {
		return strings.TrimSpace(name)
	}
	return label
}

// BuildGroups converts raw Gatus statuses into sorted groups of pills.
func BuildGroups(statuses []EndpointStatus) []Group {
	byName := map[string][]Pill{}
	for _, s := range statuses {
		group := s.Group
		if group == "" {
			group = ungroupedName
		}
		byName[group] = append(byName[group], toPill(s))
	}

	groups := make([]Group, 0, len(byName))
	for name, pills := range byName {
		sort.Slice(pills, func(i, j int) bool { return pills[i].Name < pills[j].Name })
		groups = append(groups, Group{Name: name, Endpoints: pills})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups
}

func toPill(s EndpointStatus) Pill {
	p := Pill{
		Key:    s.Key,
		Label:  Label(s.Name),
		Name:   s.Name,
		State:  Classify(s.Results),
		Recent: make([]bool, len(s.Results)),
	}
	for i, r := range s.Results {
		p.Recent[i] = r.Success
	}
	if n := len(s.Results); n > 0 {
		last := s.Results[n-1]
		p.LastDuration = last.Duration.Round(time.Millisecond).String()
		p.LastTimestamp = last.Timestamp
		p.Errors = last.Errors
	}
	return p
}
