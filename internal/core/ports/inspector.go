package ports

import (
	"context"
	"github.com/thushan/olla/internal/core/domain"
	"net/http"
)

type RequestInspector interface {
	Inspect(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error
	Name() string
}

type InspectorChain interface {
	Inspect(ctx context.Context, r *http.Request, targetPath string) (*domain.RequestProfile, error)
	AddInspector(inspector RequestInspector)
}
