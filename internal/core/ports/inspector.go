package ports

import (
	"context"
	"net/http"

	"github.com/thushan/olla/internal/core/domain"
)

type RequestInspector interface {
	Inspect(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error
	Name() string
}

type InspectorChain interface {
	Inspect(ctx context.Context, r *http.Request, targetPath string) (*domain.RequestProfile, error)
	AddInspector(inspector RequestInspector)
}
