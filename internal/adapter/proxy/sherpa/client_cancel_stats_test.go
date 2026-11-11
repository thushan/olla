package sherpa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func proxyOnceSherpa(t *testing.T, ctx context.Context, svc *Service, ep *domain.Endpoint) error {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	return svc.ProxyRequestToEndpoints(ctx, rec, req, []*domain.Endpoint{ep}, &ports.RequestStats{StartTime: time.Now()}, createTestLogger())
}

func newSherpaForCancel(t *testing.T) *Service {
	t.Helper()
	config := &Configuration{}
	config.StreamBufferSize = 8192
	svc, err := NewService(&testDiscoveryService{}, &testSelector{}, config, nil, nil, createTestLogger())
	require.NoError(t, err)
	return svc
}

// Issue #269: a client hang-up is not a backend failure in the counters.
func TestClientCancelBeforeFirstByte_NotCountedAsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	ep := &domain.Endpoint{Name: "ep", URL: u, URLString: srv.URL, Status: domain.StatusHealthy}
	svc := newSherpaForCancel(t)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_ = proxyOnceSherpa(t, ctx, svc, ep)
	cancel()

	require.Zero(t, svc.GetProxyStats().FailedRequests, "client cancel counted as a failed request")
}

// Positive control: a genuine transport failure is still counted.
func TestBackendConnectionFailure_StillCountedAsFailure(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	u, err := url.Parse(dead.URL)
	require.NoError(t, err)
	ep := &domain.Endpoint{Name: "ep", URL: u, URLString: dead.URL, Status: domain.StatusHealthy}
	dead.Close()
	svc := newSherpaForCancel(t)

	require.Error(t, proxyOnceSherpa(t, context.Background(), svc, ep))
	require.Positive(t, svc.GetProxyStats().FailedRequests)
}
