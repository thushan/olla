package domain

import "github.com/puzpuzpuz/xsync/v4"

const (
	InspectionMetaPathSupport = "path_support"
)

// RequestType identifies what kind of LLM request this is
type RequestType int

const (
	RequestTypeUnknown RequestType = iota
	RequestTypeChat
	RequestTypeCompletion
	RequestTypeEmbedding
	RequestTypeImage
)

type RequestProfile struct {
	InspectionMeta *xsync.Map[string, interface{}]

	// Rich request metadata for intelligent routing
	ModelCapabilities    *ModelCapabilities    // What the request needs
	ResourceRequirements *ResourceRequirements // Resources needed
	RoutingDecision      *ModelRoutingDecision // Routing strategy decision
	Path                 string
	ModelName            string
	SupportedBy          []string

	RequestType          RequestType // Chat, completion, embedding, etc.
	EstimatedTokens      int         // For capacity planning
	RequiresFunctionCall bool
	RequiresVision       bool
}

func NewRequestProfile(path string) *RequestProfile {
	return &RequestProfile{
		Path:           path,
		SupportedBy:    make([]string, 0, 4),
		InspectionMeta: xsync.NewMap[string, interface{}](),
	}
}

func (rp *RequestProfile) IsCompatibleWith(endpointType string) bool {
	if endpointType == ProfileAuto {
		return true
	}

	if len(rp.SupportedBy) == 0 {
		return true
	}

	for _, supported := range rp.SupportedBy {
		if supported == endpointType {
			return true
		}
		if supported == ProfileOpenAICompatible && (endpointType == ProfileOllama || endpointType == ProfileLmStudio) {
			return true
		}
	}

	return false
}

func (rp *RequestProfile) AddSupportedProfile(profileType string) {
	if profileType == "" {
		return
	}

	for _, existing := range rp.SupportedBy {
		if existing == profileType {
			return
		}
	}

	rp.SupportedBy = append(rp.SupportedBy, profileType)
}

func (rp *RequestProfile) SetInspectionMeta(key string, value interface{}) {
	rp.InspectionMeta.Store(key, value)
}
