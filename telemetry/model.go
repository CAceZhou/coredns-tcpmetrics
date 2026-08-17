package telemetry

import "time"

// Connection is a point-in-time view of one TCP socket. LossRate is derived
// from retransmitted segments and is therefore an estimate of packet loss.
type Connection struct {
	ID            string    `json:"id"`
	Family        int       `json:"family"`
	LocalAddress  string    `json:"local_address"`
	LocalPort     uint16    `json:"local_port"`
	RemoteAddress string    `json:"remote_address"`
	RemotePort    uint16    `json:"remote_port"`
	State         string    `json:"state"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	SentSegments  uint64    `json:"sent_segments"`
	Retransmits   uint64    `json:"retransmits"`
	LossRate      float64   `json:"loss_rate"`
	RTT           uint32    `json:"rtt_us,omitempty"`
	CongestionWnd uint32    `json:"congestion_window,omitempty"`
	Inode         uint32    `json:"inode,omitempty"`
}

// Snapshot is immutable once published by Store.
type Snapshot struct {
	GeneratedAt time.Time    `json:"generated_at"`
	Connections []Connection `json:"connections"`
	Error       string       `json:"error,omitempty"`
}

// Summary describes the currently retained connection set.
type Summary struct {
	GeneratedAt       time.Time `json:"generated_at"`
	Connections       int       `json:"connections"`
	Established       int       `json:"established"`
	SentSegments      uint64    `json:"sent_segments"`
	Retransmits       uint64    `json:"retransmits"`
	WeightedLossRate  float64   `json:"weighted_loss_rate"`
	LastCollectionErr string    `json:"last_collection_error,omitempty"`
}

// TCPProvider enumerates TCP sockets from an operating-system data source.
type TCPProvider interface {
	Connections() ([]Connection, error)
}
