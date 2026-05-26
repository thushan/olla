package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
)

type manualRefreshDiscoveryService struct {
	err          error
	refreshCalls int
}

func (m *manualRefreshDiscoveryService) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return nil, nil
}

func (m *manualRefreshDiscoveryService) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return nil, nil
}

func (m *manualRefreshDiscoveryService) RefreshEndpoints(ctx context.Context) error {
	m.refreshCalls++
	return m.err
}

func (m *manualRefreshDiscoveryService) UpdateEndpointStatus(ctx context.Context, endpoint *domain.Endpoint) error {
	return nil
}

func TestDiscoveryRefreshHandler_RefreshesDiscovery(t *testing.T) {
	discovery := &manualRefreshDiscoveryService{}
	app := &Application{discoveryService: discovery}

	req := httptest.NewRequest(http.MethodPost, "/internal/discovery/refresh", nil)
	w := httptest.NewRecorder()

	app.discoveryRefreshHandler(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	assert.Equal(t, 1, discovery.refreshCalls)

	var response DiscoveryRefreshResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.True(t, response.Refreshed)
	assert.False(t, response.Timestamp.IsZero())
}

func TestDiscoveryRefreshHandler_ReturnsServiceUnavailableWithoutDiscovery(t *testing.T) {
	app := &Application{}

	req := httptest.NewRequest(http.MethodPost, "/internal/discovery/refresh", nil)
	w := httptest.NewRecorder()

	app.discoveryRefreshHandler(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestDiscoveryRefreshHandler_ReturnsInternalServerErrorOnRefreshFailure(t *testing.T) {
	discovery := &manualRefreshDiscoveryService{err: errors.New("refresh failed")}
	app := &Application{discoveryService: discovery}

	req := httptest.NewRequest(http.MethodPost, "/internal/discovery/refresh", nil)
	w := httptest.NewRecorder()

	app.discoveryRefreshHandler(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, 1, discovery.refreshCalls)
}
