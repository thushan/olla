package profile

// DockerModelRunnerResponse is the response structure from Docker Model Runner /engines/v1/models endpoint.
// DMR uses the standard OpenAI-compatible list format.
type DockerModelRunnerResponse struct {
	Object string                   `json:"object"`
	Data   []DockerModelRunnerModel `json:"data"`
}

// DockerModelRunnerModel represents a single model in a DMR response.
// Model IDs follow the "namespace/name" pattern (e.g., "ai/smollm2", "docker/llama3.2").
type DockerModelRunnerModel struct {
	ID      string `json:"id"`       // namespace/name format
	Object  string `json:"object"`   // always "model"
	OwnedBy string `json:"owned_by"` // publisher or "docker" (default)
	Created int64  `json:"created"`  // unix timestamp
}
