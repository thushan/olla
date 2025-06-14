package domain

import "github.com/puzpuzpuz/xsync/v4"

const (
	InspectionMetaPathSupport = "path_support"
)

type RequestProfile struct {
	InspectionMeta *xsync.Map[string, interface{}]
	Path           string
	ModelName      string
	SupportedBy    []string
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
