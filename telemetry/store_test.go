package telemetry

import (
	"testing"
	"time"
)

type sequenceProvider struct {
	rows [][]Connection
	at   int
}

func (p *sequenceProvider) Connections() ([]Connection, error) {
	row := p.rows[p.at]
	if p.at < len(p.rows)-1 {
		p.at++
	}
	return append([]Connection(nil), row...), nil
}

func TestStoreComputesWindowLossAndRetainsClosedConnections(t *testing.T) {
	provider := &sequenceProvider{rows: [][]Connection{{{ID: "one", State: "ESTABLISHED", SentSegments: 100, Retransmits: 2}}, {{ID: "one", State: "ESTABLISHED", SentSegments: 120, Retransmits: 3}}, {}}}
	store := NewStore(provider, time.Second, time.Minute)
	store.collect()
	store.collect()
	snapshot := store.Snapshot()
	if got := snapshot.Connections[0].LossRate; got != 0.05 {
		t.Fatalf("loss rate = %v, want .05", got)
	}
	firstSeen := snapshot.Connections[0].FirstSeen
	store.collect()
	snapshot = store.Snapshot()
	if len(snapshot.Connections) != 1 || !snapshot.Connections[0].FirstSeen.Equal(firstSeen) {
		t.Fatalf("closed connection was not retained: %#v", snapshot)
	}
}

func TestDeltaHandlesCounterReset(t *testing.T) {
	if got := delta(100, 4); got != 4 {
		t.Fatalf("delta=%d, want 4", got)
	}
}
