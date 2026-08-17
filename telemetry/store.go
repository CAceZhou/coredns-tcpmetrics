package telemetry

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Store periodically samples a provider and retains recently closed sockets.
type Store struct {
	provider  TCPProvider
	interval  time.Duration
	retention time.Duration

	mu       sync.RWMutex
	snapshot Snapshot
	previous map[string]Connection
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewStore(provider TCPProvider, interval, retention time.Duration) *Store {
	return &Store{provider: provider, interval: interval, retention: retention, previous: make(map[string]Connection)}
}

func (s *Store) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.collect()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.collect()
			}
		}
	}()
}

func (s *Store) Stop() {
	if s.cancel != nil {
		s.cancel()
		s.wg.Wait()
	}
}

func (s *Store) collect() {
	now := time.Now().UTC()
	rows, err := s.provider.Connections()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.snapshot.GeneratedAt = now
		s.snapshot.Error = err.Error()
		return
	}
	seen := make(map[string]bool, len(rows))
	for i := range rows {
		row := &rows[i]
		seen[row.ID] = true
		row.LastSeen = now
		if old, ok := s.previous[row.ID]; ok {
			row.FirstSeen = old.FirstSeen
			dSent, dRetrans := delta(old.SentSegments, row.SentSegments), delta(old.Retransmits, row.Retransmits)
			if dSent > 0 {
				row.LossRate = float64(dRetrans) / float64(dSent)
			}
		} else {
			row.FirstSeen = now
			if row.SentSegments > 0 {
				row.LossRate = float64(row.Retransmits) / float64(row.SentSegments)
			}
		}
		s.previous[row.ID] = *row
	}
	for id, old := range s.previous {
		if seen[id] {
			continue
		}
		if now.Sub(old.LastSeen) <= s.retention {
			rows = append(rows, old)
		} else {
			delete(s.previous, id)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	s.snapshot = Snapshot{GeneratedAt: now, Connections: rows}
}

func delta(old, current uint64) uint64 {
	if current >= old {
		return current - old
	}
	return current
}

func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.snapshot
	out.Connections = append([]Connection(nil), s.snapshot.Connections...)
	return out
}

func (s *Store) Summary() Summary {
	snap := s.Snapshot()
	result := Summary{GeneratedAt: snap.GeneratedAt, Connections: len(snap.Connections), LastCollectionErr: snap.Error}
	for _, c := range snap.Connections {
		if c.State == "ESTABLISHED" {
			result.Established++
		}
		result.SentSegments += c.SentSegments
		result.Retransmits += c.Retransmits
	}
	if result.SentSegments > 0 {
		result.WeightedLossRate = float64(result.Retransmits) / float64(result.SentSegments)
	}
	return result
}
