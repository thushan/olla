package profile

// LMDeployResponse is the response structure from LMDeploy /v1/models endpoint.
// The shape follows the OpenAI ModelList format but with LMDeploy-specific field
// values — notably owned_by defaults to "lmdeploy" and there is no max_model_len.
type LMDeployResponse struct {
	Object string          `json:"object"`
	Data   []LMDeployModel `json:"data"`
}

// LMDeployModel represents a single model entry in the LMDeploy /v1/models response.
// Key difference from vLLM: no max_model_len field. Key difference from vLLM/SGLang:
// owned_by defaults to "lmdeploy" (not "vllm" or "sglang").
type LMDeployModel struct {
	Root       *string                  `json:"root,omitempty"`
	Parent     *string                  `json:"parent,omitempty"`
	ID         string                   `json:"id"`
	Object     string                   `json:"object"`
	OwnedBy    string                   `json:"owned_by"`
	Permission []LMDeployModelPermission `json:"permission,omitempty"`
	Created    int64                    `json:"created"`
}

// LMDeployModelPermission mirrors the OpenAI permission shape that LMDeploy exposes.
type LMDeployModelPermission struct {
	Group              *string `json:"group"`
	ID                 string  `json:"id"`
	Object             string  `json:"object"`
	Organization       string  `json:"organization"`
	Created            int64   `json:"created"`
	AllowCreateEngine  bool    `json:"allow_create_engine"`
	AllowSampling      bool    `json:"allow_sampling"`
	AllowLogprobs      bool    `json:"allow_logprobs"`
	AllowSearchIndices bool    `json:"allow_search_indices"`
	AllowView          bool    `json:"allow_view"`
	AllowFineTuning    bool    `json:"allow_fine_tuning"`
	IsBlocking         bool    `json:"is_blocking"`
}
