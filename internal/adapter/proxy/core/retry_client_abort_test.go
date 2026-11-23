package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// A client cancel racing a fresh-connection EOF leaves the EOF in the error
// chain. The retry loop must not read that as the backend dying: no demotion
// and no attempt on a second endpoint.
func TestExecuteWithRetry_ClientAbortWithEOF_NoDemotionNoFailover(t *testing.T) {
	t.Parallel()

	ds := &testDiscoveryService{}
	h := newCapturingRetryHandler(t, ds)

	// Cancel from inside the first attempt: the loop's top-of-iteration check
	// would otherwise stop the request before any attempt ran.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var attempts []string
	proxyFunc := func(c context.Context, w http.ResponseWriter, r *http.Request, ep *domain.Endpoint, s *ports.RequestStats) error {
		attempts = append(attempts, ep.Name)
		cancel()
		return fmt.Errorf("Post: %w", io.EOF)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	err := h.ExecuteWithRetry(ctx, httptest.NewRecorder(), req,
		[]*domain.Endpoint{namedEndpoint("a"), namedEndpoint("b")},
		&roundRobinSelector{}, &ports.RequestStats{}, proxyFunc)

	require.Error(t, err)
	assert.Equal(t, []string{"a"}, attempts, "must not fail over for a client that has gone")
	assert.Nil(t, ds.updatedEndpoint, "client abort must not demote the endpoint")
}
