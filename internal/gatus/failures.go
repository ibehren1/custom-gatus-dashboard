package gatus

import (
	"sort"
	"strings"
	"time"
)

// Incident states as shown in the failure list.
const (
	IncidentDown     = "down"
	IncidentFlapping = "flapping"
	IncidentResolved = "resolved"
)

// Incident is a run of consecutive failed checks for one endpoint.
type Incident struct {
	State    string    `json:"state"`
	Key      string    `json:"key"`
	Group    string    `json:"group"`
	Name     string    `json:"name"`
	Label    string    `json:"label"`
	Start    time.Time `json:"start"`
	Last     time.Time `json:"last"`
	Resolved time.Time `json:"resolved,omitzero"`
	Count    int       `json:"count"`
	Reason   string    `json:"reason"`
}

// Ongoing reports whether the endpoint is still failing.
func (i Incident) Ongoing() bool { return i.Resolved.IsZero() }

// FailureLog turns the stream of check results into incidents. It keeps all
// ongoing incidents plus the most recent resolved ones, up to maxResolved and
// resolved no longer than maxAge ago. FailureLog is not safe for concurrent use.
type FailureLog struct {
	maxResolved int
	maxAge      time.Duration

	lastSeen  map[string]time.Time
	open      map[string]*Incident
	incidents []*Incident
}

func NewFailureLog(maxResolved int, maxAge time.Duration) *FailureLog {
	return &FailureLog{
		maxResolved: maxResolved,
		maxAge:      maxAge,
		lastSeen:    map[string]time.Time{},
		open:        map[string]*Incident{},
	}
}

// Observe processes results not seen before. Results must be ordered oldest first,
// as returned by Gatus.
func (l *FailureLog) Observe(statuses []EndpointStatus, now time.Time) {
	present := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		present[s.Key] = true
		seen := l.lastSeen[s.Key]
		for _, r := range s.Results {
			if !r.Timestamp.After(seen) {
				continue
			}
			l.record(s, r)
			l.lastSeen[s.Key] = r.Timestamp
		}
	}

	// Forget endpoints that were removed from the Gatus config.
	for key := range l.lastSeen {
		if !present[key] {
			delete(l.lastSeen, key)
			delete(l.open, key)
		}
	}
	l.prune(present, now)
}

func (l *FailureLog) record(s EndpointStatus, r Result) {
	inc := l.open[s.Key]
	if r.Success {
		if inc != nil {
			inc.Resolved = r.Timestamp
			delete(l.open, s.Key)
		}
		return
	}
	if inc == nil {
		inc = &Incident{
			Key:   s.Key,
			Group: s.Group,
			Name:  s.Name,
			Label: Label(s.Name),
			Start: r.Timestamp,
		}
		l.open[s.Key] = inc
		l.incidents = append(l.incidents, inc)
	}
	inc.Last = r.Timestamp
	inc.Count++
	inc.Reason = failureReason(r)
}

func (l *FailureLog) prune(present map[string]bool, now time.Time) {
	cutoff := now.Add(-l.maxAge)
	resolved := 0
	keep := make([]bool, len(l.incidents))
	// Walk newest first so the most recent resolved incidents survive the cap.
	for i := len(l.incidents) - 1; i >= 0; i-- {
		inc := l.incidents[i]
		switch {
		case !present[inc.Key]:
		case inc.Ongoing():
			keep[i] = true
		case inc.Resolved.Before(cutoff) || resolved >= l.maxResolved:
		default:
			resolved++
			keep[i] = true
		}
	}
	kept := make([]*Incident, 0, len(l.incidents))
	for i, inc := range l.incidents {
		if keep[i] {
			kept = append(kept, inc)
		}
	}
	l.incidents = kept
}

// Snapshot returns incidents most recent first, by when they started (still
// down) or recovered. pillStates maps endpoint key to its current pill state;
// an endpoint's latest resolved incident is reported as flapping while its pill is.
func (l *FailureLog) Snapshot(pillStates map[string]string) []Incident {
	latest := make(map[string]*Incident, len(l.incidents))
	for _, inc := range l.incidents {
		latest[inc.Key] = inc
	}

	out := make([]Incident, len(l.incidents))
	for i, inc := range l.incidents {
		out[i] = *inc
		switch {
		case inc.Ongoing():
			out[i].State = IncidentDown
		case latest[inc.Key] == inc && pillStates[inc.Key] == StateFlapping:
			out[i].State = IncidentFlapping
		default:
			out[i].State = IncidentResolved
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].latestEvent().After(out[j].latestEvent())
	})
	return out
}

// latestEvent is when the endpoint went down, or when it recovered if it has.
func (i Incident) latestEvent() time.Time {
	if i.Ongoing() {
		return i.Start
	}
	return i.Resolved
}

// failureReason summarizes why a check failed: errors if any, otherwise the
// conditions that did not pass.
func failureReason(r Result) string {
	if len(r.Errors) > 0 {
		return strings.Join(r.Errors, "; ")
	}
	var failed []string
	for _, c := range r.ConditionResults {
		if !c.Success {
			failed = append(failed, c.Condition)
		}
	}
	return strings.Join(failed, ", ")
}
