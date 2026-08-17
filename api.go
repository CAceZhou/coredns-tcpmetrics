package tcpmetrics

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/CAceZhou/coredns-tcpmetrics/telemetry"
)

func (s *Service) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/tcp/connections", s.handleConnections)
	mux.HandleFunc("/v1/tcp/connections/", s.handleConnection)
	mux.HandleFunc("/v1/tcp/summary", s.handleSummary)
	return s.authenticate(mux)
}

func (s *Service) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.Token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Service) handleConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	snap := s.store.Snapshot()
	rows := snap.Connections
	state := strings.ToUpper(r.URL.Query().Get("state"))
	pattern := r.URL.Query().Get("match")
	family, ok := optionalInt(r.URL.Query().Get("family"), 4, 6)
	if !ok {
		writeError(w, http.StatusBadRequest, "family must be 4 or 6")
		return
	}
	localPorts, ok := portSet(r.URL.Query().Get("local_port"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid local_port")
		return
	}
	remotePorts, ok := portSet(r.URL.Query().Get("remote_port"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid remote_port")
		return
	}
	localCIDR, ok := parseCIDR(r.URL.Query().Get("local_cidr"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid local_cidr")
		return
	}
	remoteCIDR, ok := parseCIDR(r.URL.Query().Get("remote_cidr"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid remote_cidr")
		return
	}
	var re *regexp.Regexp
	if pattern != "" {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid match expression")
			return
		}
	}
	filtered := rows[:0]
	for _, row := range rows {
		key := row.ID + " " + row.LocalAddress + " " + row.RemoteAddress
		if state != "" && row.State != state {
			continue
		}
		if family != 0 && connectionFamily(row) != family {
			continue
		}
		if len(localPorts) > 0 && !localPorts[row.LocalPort] {
			continue
		}
		if len(remotePorts) > 0 && !remotePorts[row.RemotePort] {
			continue
		}
		if localCIDR != nil && !localCIDR.Contains(net.ParseIP(row.LocalAddress)) {
			continue
		}
		if remoteCIDR != nil && !remoteCIDR.Contains(net.ParseIP(row.RemoteAddress)) {
			continue
		}
		if re != nil && !re.MatchString(key) {
			continue
		}
		filtered = append(filtered, row)
	}
	offset, ok := queryInt(r, "offset", 0, 0, 1_000_000)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid offset")
		return
	}
	limit, ok := queryInt(r, "limit", 100, 1, 1000)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	if offset > len(filtered) {
		offset = len(filtered)
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	writeJSON(w, http.StatusOK, map[string]any{"generated_at": snap.GeneratedAt, "error": snap.Error, "total": len(filtered), "connections": filtered[offset:end]})
}

func optionalInt(raw string, allowed ...int) (int, bool) {
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	for _, value := range allowed {
		if n == value {
			return n, true
		}
	}
	return 0, false
}

func portSet(raw string) (map[uint16]bool, bool) {
	result := map[uint16]bool{}
	if raw == "" {
		return result, true
	}
	for _, part := range strings.Split(raw, ",") {
		n, err := strconv.ParseUint(part, 10, 16)
		if err != nil || n == 0 {
			return nil, false
		}
		result[uint16(n)] = true
	}
	return result, true
}

func parseCIDR(raw string) (*net.IPNet, bool) {
	if raw == "" {
		return nil, true
	}
	_, network, err := net.ParseCIDR(raw)
	return network, err == nil
}

func connectionFamily(row telemetry.Connection) int {
	if row.Family == 4 || row.Family == 6 {
		return row.Family
	}
	ip := net.ParseIP(row.RemoteAddress)
	if ip != nil && ip.To4() != nil {
		return 4
	}
	if ip != nil {
		return 6
	}
	return 0
}

func (s *Service) handleConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/tcp/connections/")
	if id == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	for _, row := range s.store.Snapshot().Connections {
		if row.ID == id {
			writeJSON(w, http.StatusOK, row)
			return
		}
	}
	writeError(w, http.StatusNotFound, "connection not found")
}

func (s *Service) handleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, s.store.Summary())
}

func queryInt(r *http.Request, name string, def, min, max int) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil && n >= min && n <= max
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
