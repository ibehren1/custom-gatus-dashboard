package gatus

import (
	"sort"
	"strings"
	"time"
)

// Incident is a run of consecutive failed checks for one endpoint.
type Incident struct {
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
// no older than maxAge. FailureLog is not safe for concurrent use.
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

// Snapshot returns ongoing incidents first (most recent failure first), then
// resolved incidents (most recently resolved first).
func (l *FailureLog) Snapshot() []Incident {
	out := make([]Incident, len(l.incidents))
	for i, inc := range l.incidents {
		out[i] = *inc
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Ongoing() != b.Ongoing() {
			return a.Ongoing()
		}
		if a.Ongoing() {
			return a.Start.After(b.Start)
		}
		return a.Resolved.After(b.Resolved)
	})
	return out
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
