package inspector

import "fmt"

type InspectionError struct {
	Err           error
	InspectorName string
	Path          string
}

func (e *InspectionError) Error() string {
	return fmt.Sprintf("inspection failed for inspector '%s' on path '%s': %v", e.InspectorName, e.Path, e.Err)
}

func (e *InspectionError) Unwrap() error {
	return e.Err
}

func NewInspectionError(inspectorName, path string, err error) *InspectionError {
	return &InspectionError{
		InspectorName: inspectorName,
		Path:          path,
		Err:           err,
	}
}

type UnsupportedPathError struct {
	Path              string
	AvailableProfiles []string
}

func (e *UnsupportedPathError) Error() string {
	return fmt.Sprintf("path '%s' not supported by any available profiles: %v", e.Path, e.AvailableProfiles)
}

func NewUnsupportedPathError(path string, availableProfiles []string) *UnsupportedPathError {
	return &UnsupportedPathError{
		Path:              path,
		AvailableProfiles: availableProfiles,
	}
}
