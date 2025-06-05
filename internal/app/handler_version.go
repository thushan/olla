package app

import (
	"encoding/json"
	"net/http"
	"runtime"

	"github.com/thushan/olla/internal/version"
)

type VersionResponse struct {
	API               APIInfo           `json:"api"`
	Links             map[string]string `json:"links"`
	Build             BuildInfo         `json:"build"`
	Name              string            `json:"name"`
	Version           string            `json:"version"`
	Edition           string            `json:"edition"`
	Description       string            `json:"description"`
	Capabilities      []string          `json:"capabilities"`
	SupportedBackends []string          `json:"supported_backends"`
}

type BuildInfo struct {
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

type APIInfo struct {
	Endpoints map[string]string `json:"endpoints"`
	Version   string            `json:"version"`
}

// versionHandler handles version requests with metadata about the application.
func (a *Application) versionHandler(w http.ResponseWriter, r *http.Request) {
	versionInfo := VersionResponse{
		Name:        version.Name,
		Version:     version.Version,
		Edition:     version.Edition,
		Description: version.Description,
		Build: BuildInfo{
			Commit:    version.Commit,
			Date:      version.Date,
			GoVersion: version.Runtime,
			Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		},
		Capabilities:      version.Capabilities,
		SupportedBackends: version.SupportedBackends,
		API: APIInfo{
			Version: "v1",
			Endpoints: map[string]string{
				"health":  "/internal/health",
				"status":  "/internal/status",
				"process": "/internal/process",
				"version": "/internal/version",
			},
		},
		Links: map[string]string{
			"homepage":      version.GithubHomeUri,
			"documentation": version.GithubHomeUri + "#readme",
			"releases":      version.GithubLatestUri,
		},
	}

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(versionInfo)
}
