package gatus

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func at(sec int, success bool) Result {
	r := Result{Success: success, Timestamp: t0.Add(time.Duration(sec) * time.Second)}
	if !success {
		r.ConditionResults = []ConditionResult{{Condition: "[CONNECTED] (false) == true"}}
	}
	return r
}

func TestFailureLogGroupsConsecutiveFailures(t *testing.T) {
	log := NewFailureLog(10, time.Hour)
	ep := EndpointStatus{Name: "NAS - tcp://nas:22", Group: "Infra", Key: "infra_nas"}

	ep.Results = []Result{at(0, true), at(10, false), at(20, false)}
	log.Observe([]EndpointStatus{ep}, t0.Add(20*time.Second))

	// Overlapping window: results at 10s and 20s must not be counted twice.
	ep.Results = []Result{at(10, false), at(20, false), at(30, false), at(40, true), at(50, false)}
	log.Observe([]EndpointStatus{ep}, t0.Add(50*time.Second))

	got := log.Snapshot()
	if len(got) != 2 {
		t.Fatalf("got %d incidents, want 2: %+v", len(got), got)
	}
	ongoing, resolved := got[0], got[1]
	if !ongoing.Ongoing() || ongoing.Count != 1 || !ongoing.Start.Equal(t0.Add(50*time.Second)) {
		t.Errorf("unexpected ongoing incident: %+v", ongoing)
	}
	if resolved.Ongoing() || resolved.Count != 3 || !resolved.Resolved.Equal(t0.Add(40*time.Second)) {
		t.Errorf("unexpected resolved incident: %+v", resolved)
	}
	if resolved.Label != "NAS" || resolved.Reason != "[CONNECTED] (false) == true" {
		t.Errorf("unexpected label/reason: %+v", resolved)
	}
}

func TestFailureLogPrunes(t *testing.T) {
	log := NewFailureLog(2, time.Hour)
	ep := EndpointStatus{Name: "a", Key: "a"}
	gone := EndpointStatus{Name: "b", Key: "b", Results: []Result{at(0, false)}}

	// Four resolved incidents for a, one ongoing for b.
	for i := 0; i < 4; i++ {
		ep.Results = append(ep.Results, at(i*20, false), at(i*20+10, true))
	}
	log.Observe([]EndpointStatus{ep, gone}, t0.Add(80*time.Second))

	got := log.Snapshot()
	if len(got) != 3 || got[0].Key != "b" {
		t.Fatalf("want ongoing b plus 2 newest resolved, got %+v", got)
	}
	if !got[1].Resolved.Equal(t0.Add(70*time.Second)) || !got[2].Resolved.Equal(t0.Add(50*time.Second)) {
		t.Errorf("kept wrong resolved incidents: %+v", got[1:])
	}

	// Removing b from Gatus drops its incident; aging out drops the rest.
	log.Observe([]EndpointStatus{ep}, t0.Add(2*time.Hour))
	if got := log.Snapshot(); len(got) != 0 {
		t.Errorf("want empty log, got %+v", got)
	}
}

func TestFailureReasonPrefersErrors(t *testing.T) {
	r := Result{Errors: []string{"timeout"}, ConditionResults: []ConditionResult{{Condition: "x"}}}
	if got := failureReason(r); got != "timeout" {
		t.Errorf("failureReason() = %q, want %q", got, "timeout")
	}
}
