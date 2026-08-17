package tcpmetrics

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
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
