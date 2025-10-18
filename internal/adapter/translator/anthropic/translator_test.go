package anthropic

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/constants"
)

// TestGetAPIPath tests the PathProvider interface implementation
func TestGetAPIPath(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())

	path := translator.GetAPIPath()
	assert.Equal(t, "/olla/anthropic/v1/messages", path, "should return the correct Anthropic API path")
}

// TestWriteError tests the ErrorWriter interface implementation
func TestWriteError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		statusCode     int
		expectedType   string
		expectedStatus int
	}{
		{
			name:           "bad_request",
			err:            errors.New("invalid model specified"),
			statusCode:     http.StatusBadRequest,
			expectedType:   "invalid_request_error",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "unauthorized",
			err:            errors.New("missing API key"),
			statusCode:     http.StatusUnauthorized,
			expectedType:   "authentication_error",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "forbidden",
			err:            errors.New("insufficient permissions"),
			statusCode:     http.StatusForbidden,
			expectedType:   "permission_error",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "not_found",
			err:            errors.New("model not found"),
			statusCode:     http.StatusNotFound,
			expectedType:   "not_found_error",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "rate_limit",
			err:            errors.New("rate limit exceeded"),
			statusCode:     http.StatusTooManyRequests,
			expectedType:   "rate_limit_error",
			expectedStatus: http.StatusTooManyRequests,
		},
		{
			name:           "service_unavailable",
			err:            errors.New("service overloaded"),
			statusCode:     http.StatusServiceUnavailable,
			expectedType:   "overloaded_error",
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:           "generic_error",
			err:            errors.New("something went wrong"),
			statusCode:     http.StatusInternalServerError,
			expectedType:   "api_error",
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(createTestLogger(), createTestConfig())
			rec := httptest.NewRecorder()

			translator.WriteError(rec, tt.err, tt.statusCode)

			// Verify status code
			assert.Equal(t, tt.expectedStatus, rec.Code)

			// Verify content type
			assert.Equal(t, constants.ContentTypeJSON, rec.Header().Get(constants.HeaderContentType))

			// Verify response body structure
			var response map[string]interface{}
			err := json.Unmarshal(rec.Body.Bytes(), &response)
			require.NoError(t, err)

			// Check error type wrapper
			assert.Equal(t, "error", response["type"])

			// Check error details
			errorObj, ok := response["error"].(map[string]interface{})
			require.True(t, ok, "error field should be an object")

			assert.Equal(t, tt.expectedType, errorObj["type"], "error type should match")
			assert.Equal(t, tt.err.Error(), errorObj["message"], "error message should match")
		})
	}
}

// TestWriteError_ErrorFormat tests Anthropic error format compliance
func TestWriteError_ErrorFormat(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())
	rec := httptest.NewRecorder()

	testErr := errors.New("test error message")
	translator.WriteError(rec, testErr, http.StatusBadRequest)

	var response map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify Anthropic error format structure
	// According to https://docs.anthropic.com/claude/reference/errors
	assert.Contains(t, response, "type", "response should have type field")
	assert.Contains(t, response, "error", "response should have error field")

	errorObj := response["error"].(map[string]interface{})
	assert.Contains(t, errorObj, "type", "error object should have type field")
	assert.Contains(t, errorObj, "message", "error object should have message field")
}

// TestName tests the Name method
func TestName(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())
	assert.Equal(t, "anthropic", translator.Name())
}

// TestWriteError_JSONEncodingFailure tests handling of JSON encoding errors
// This test is mostly for coverage, as encoding errors are rare
func TestWriteError_JSONEncodingSuccess(t *testing.T) {
	translator := NewTranslator(createTestLogger(), createTestConfig())
	rec := httptest.NewRecorder()

	// Standard error that should encode successfully
	translator.WriteError(rec, errors.New("test"), http.StatusBadRequest)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.NotEmpty(t, rec.Body.Bytes(), "response body should not be empty")

	// Verify it's valid JSON
	var response map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err, "response should be valid JSON")
}
