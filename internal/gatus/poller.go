package gatus

import (
	"context"
	"log"
	"sync"
	"time"
)

// seedResultCount is how many results per endpoint are fetched on the first
// poll to backfill the failure log. Gatus caps pageSize at 100.
const seedResultCount = 100

// Poller periodically fetches statuses from Gatus so the dashboard and the
// failure log stay current regardless of how many browsers are viewing.
type Poller struct {
	client      *Client
	interval    time.Duration
	historySize int

	mu        sync.Mutex
	statuses  []EndpointStatus
	fetchedAt time.Time
	lastErr   error
	failures  *FailureLog
}

func NewPoller(client *Client, interval time.Duration, historySize int, failures *FailureLog) *Poller {
	return &Poller{
		client:      client,
		interval:    interval,
		historySize: historySize,
		failures:    failures,
	}
}

// Run polls until ctx is canceled.
func (p *Poller) Run(ctx context.Context) {
	p.poll(ctx)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *Poller) poll(ctx context.Context) {
	resultCount := p.historySize
	if p.fetchedAt.IsZero() {
		resultCount = seedResultCount
	}
	statuses, err := p.client.Statuses(ctx, resultCount)

	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		if p.lastErr == nil {
			log.Printf("fetch statuses: %v", err)
		}
		p.lastErr = err
		return
	}
	if p.lastErr != nil {
		log.Printf("fetch statuses: recovered")
	}
	p.lastErr = nil
	p.statuses, p.fetchedAt = statuses, time.Now()
	p.failures.Observe(statuses, p.fetchedAt)
}

// Snapshot returns the latest data. ok is false until the first successful poll;
// stale is true when the most recent poll failed.
func (p *Poller) Snapshot() (groups []Group, failures []Incident, fetchedAt time.Time, stale, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.statuses == nil {
		return nil, nil, time.Time{}, false, false
	}
	return BuildGroups(p.statuses, p.historySize), p.failures.Snapshot(), p.fetchedAt, p.lastErr != nil, true
}
