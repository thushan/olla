package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
)

func TestProxyHandler_RejectsModelOutsideAllowlist(t *testing.T) {
	styledLog := &mockStyledLogger{}
	profileFactory, err := profile.NewFactoryWithDefaults()
	require.NoError(t, err)

	inspectorFactory := inspector.NewFactory(profileFactory, styledLog)
	chain := inspectorFactory.CreateChain()
	chain.AddInspector(inspectorFactory.CreatePathInspector())
	bodyInspector, err := inspectorFactory.CreateBodyInspector()
	require.NoError(t, err)
	chain.AddInspector(bodyInspector)

	cfg := config.DefaultConfig()
	cfg.AllowedModels = []string{"qwen3-coder-30b"}
	app := &Application{
		Config:         cfg,
		logger:         styledLog,
		inspectorChain: chain,
		allowedModels:  makeAllowedModelSet(cfg.AllowedModels),
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/olla/proxy/v1/chat/completions",
		bytes.NewBufferString(`{"model":"unknown-model","messages":[{"role":"user","content":"hi"}]}`),
	)
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w := httptest.NewRecorder()

	app.proxyHandler(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Header().Get(constants.HeaderContentType), constants.ContentTypeJSON)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "unknown-model", body["model"])
	assert.Contains(t, body["error"], "not allowed")
}
