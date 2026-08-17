package tcpmetrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CAceZhou/coredns-tcpmetrics/telemetry"
)

type staticProvider struct{ rows []telemetry.Connection }

func (p staticProvider) Connections() ([]telemetry.Connection, error) {
	return append([]telemetry.Connection(nil), p.rows...), nil
}

func testService(t *testing.T) *Service {
	t.Helper()
	store := telemetry.NewStore(staticProvider{[]telemetry.Connection{{ID: "one", State: "ESTABLISHED", RemoteAddress: "192.0.2.1", RemotePort: 443, SentSegments: 10, Retransmits: 1}}}, time.Hour, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	store.Start(ctx)
	t.Cleanup(func() { cancel(); store.Stop() })
	return &Service{cfg: Config{Token: "secret"}, store: store}
}

func TestAPIAuthenticationAndFiltering(t *testing.T) {
	svc := testService(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/tcp/connections?state=ESTABLISHED&match=192%5C.0%5C.2%5C.1", nil)
	response := httptest.NewRecorder()
	svc.routes().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("without token status=%d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/tcp/connections?state=ESTABLISHED&match=192%5C.0%5C.2%5C.1", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	svc.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("with token status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAPIRejectsInvalidPagination(t *testing.T) {
	svc := testService(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/tcp/connections?limit=0", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	svc.routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", response.Code)
	}
}
