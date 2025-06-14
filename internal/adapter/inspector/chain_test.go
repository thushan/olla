package inspector

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestNewChain(t *testing.T) {
	logger := createTestLogger()
	chain := NewChain(logger)

	if chain == nil {
		t.Fatal("NewChain() should not return nil")
	}

	if len(chain.inspectors) != 0 {
		t.Errorf("NewChain() should start with empty inspectors, got %d", len(chain.inspectors))
	}
}

func TestChain_AddInspector(t *testing.T) {
	logger := createTestLogger()
	chain := NewChain(logger)

	inspector1 := &mockInspector{name: "test1"}
	inspector2 := &mockInspector{name: "test2"}

	chain.AddInspector(inspector1)
	if len(chain.inspectors) != 1 {
		t.Errorf("AddInspector() should add inspector, got %d inspectors", len(chain.inspectors))
	}

	chain.AddInspector(inspector2)
	if len(chain.inspectors) != 2 {
		t.Errorf("AddInspector() should add second inspector, got %d inspectors", len(chain.inspectors))
	}

	if chain.inspectors[0] != inspector1 {
		t.Error("AddInspector() should maintain order")
	}
	if chain.inspectors[1] != inspector2 {
		t.Error("AddInspector() should maintain order")
	}
}

func TestChain_Inspect_EmptyChain(t *testing.T) {
	logger := createTestLogger()
	chain := NewChain(logger)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	targetPath := "/v1/chat/completions"

	profile, err := chain.Inspect(ctx, req, targetPath)

	if err != nil {
		t.Errorf("Inspect() with empty chain should not error, got %v", err)
	}

	if profile == nil {
		t.Fatal("Inspect() should return a profile, got nil")
	}

	if profile.Path != targetPath {
		t.Errorf("Inspect() profile path = %v, want %v", profile.Path, targetPath)
	}

	if len(profile.SupportedBy) != 0 {
		t.Errorf("Inspect() with empty chain should have no supported profiles, got %v", profile.SupportedBy)
	}
}

func TestChain_Inspect_SingleInspector_Success(t *testing.T) {
	logger := createTestLogger()
	chain := NewChain(logger)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	targetPath := "/v1/chat/completions"

	inspector := &mockInspector{
		name: "test",
		inspectFunc: func(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error {
			profile.AddSupportedProfile("test-profile")
			profile.SetInspectionMeta("test-key", "test-value")
			return nil
		},
	}

	chain.AddInspector(inspector)
	profile, err := chain.Inspect(ctx, req, targetPath)

	if err != nil {
		t.Errorf("Inspect() should not error, got %v", err)
	}

	if !contains(profile.SupportedBy, "test-profile") {
		t.Errorf("Inspect() should include test-profile, got %v", profile.SupportedBy)
	}

	value, exists := profile.InspectionMeta.Load("test-key")
	if !exists || value != "test-value" {
		t.Errorf("Inspect() should set metadata, got %v", value)
	}

	if !inspector.called {
		t.Error("Inspect() should call inspector")
	}
}

func TestChain_Inspect_MultipleInspectors_AllSuccess(t *testing.T) {
	logger := createTestLogger()
	chain := NewChain(logger)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	targetPath := "/v1/chat/completions"

	inspector1 := &mockInspector{
		name: "inspector1",
		inspectFunc: func(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error {
			profile.AddSupportedProfile("profile1")
			return nil
		},
	}

	inspector2 := &mockInspector{
		name: "inspector2",
		inspectFunc: func(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error {
			profile.AddSupportedProfile("profile2")
			profile.SetInspectionMeta("inspector2", "data")
			return nil
		},
	}

	chain.AddInspector(inspector1)
	chain.AddInspector(inspector2)

	profile, err := chain.Inspect(ctx, req, targetPath)

	if err != nil {
		t.Errorf("Inspect() should not error, got %v", err)
	}

	if !contains(profile.SupportedBy, "profile1") {
		t.Errorf("Inspect() should include profile1, got %v", profile.SupportedBy)
	}

	if !contains(profile.SupportedBy, "profile2") {
		t.Errorf("Inspect() should include profile2, got %v", profile.SupportedBy)
	}

	value, exists := profile.InspectionMeta.Load("inspector2")
	if !exists || value != "data" {
		t.Errorf("Inspect() should preserve metadata from all inspectors")
	}

	if !inspector1.called || !inspector2.called {
		t.Error("Inspect() should call all inspectors")
	}
}

func TestChain_Inspect_InspectorFailure_ContinuesChain(t *testing.T) {
	logger := createTestLogger()
	chain := NewChain(logger)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	targetPath := "/v1/chat/completions"

	failingInspector := &mockInspector{
		name: "failing",
		inspectFunc: func(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error {
			return errors.New("inspector failure")
		},
	}

	successInspector := &mockInspector{
		name: "success",
		inspectFunc: func(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error {
			profile.AddSupportedProfile("success-profile")
			return nil
		},
	}

	chain.AddInspector(failingInspector)
	chain.AddInspector(successInspector)

	profile, err := chain.Inspect(ctx, req, targetPath)

	if err != nil {
		t.Errorf("Inspect() should not error when individual inspector fails, got %v", err)
	}

	if !contains(profile.SupportedBy, "success-profile") {
		t.Errorf("Inspect() should continue chain after failure, got %v", profile.SupportedBy)
	}

	if !failingInspector.called || !successInspector.called {
		t.Error("Inspect() should call all inspectors even after failure")
	}
}

func TestChain_Inspect_AllInspectorsFail_ReturnsProfile(t *testing.T) {
	logger := createTestLogger()
	chain := NewChain(logger)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	targetPath := "/v1/chat/completions"

	inspector1 := &mockInspector{
		name: "fail1",
		inspectFunc: func(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error {
			return errors.New("failure 1")
		},
	}

	inspector2 := &mockInspector{
		name: "fail2",
		inspectFunc: func(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error {
			return errors.New("failure 2")
		},
	}

	chain.AddInspector(inspector1)
	chain.AddInspector(inspector2)

	profile, err := chain.Inspect(ctx, req, targetPath)

	if err != nil {
		t.Errorf("Inspect() should not error when all inspectors fail, got %v", err)
	}

	if profile == nil {
		t.Fatal("Inspect() should return profile even when all inspectors fail")
	}

	if profile.Path != targetPath {
		t.Errorf("Inspect() profile path = %v, want %v", profile.Path, targetPath)
	}

	if len(profile.SupportedBy) != 0 {
		t.Errorf("Inspect() should have no supported profiles when all fail, got %v", profile.SupportedBy)
	}
}

func TestChain_Inspect_NilContext(t *testing.T) {
	logger := createTestLogger()
	chain := NewChain(logger)
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	targetPath := "/v1/chat/completions"

	inspector := &mockInspector{
		name: "test",
		inspectFunc: func(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error {
			if ctx == nil {
				return errors.New("context is nil")
			}
			return nil
		},
	}

	chain.AddInspector(inspector)

	// This should not panic even with nil context
	profile, err := chain.Inspect(nil, req, targetPath)

	if err != nil {
		t.Errorf("Inspect() should handle nil context gracefully, got %v", err)
	}

	if profile == nil {
		t.Fatal("Inspect() should return profile with nil context")
	}
}

func TestChain_Inspect_ExecutionOrder(t *testing.T) {
	logger := createTestLogger()
	chain := NewChain(logger)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	targetPath := "/v1/chat/completions"

	var executionOrder []string

	inspector1 := &mockInspector{
		name: "first",
		inspectFunc: func(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error {
			executionOrder = append(executionOrder, "first")
			return nil
		},
	}

	inspector2 := &mockInspector{
		name: "second",
		inspectFunc: func(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error {
			executionOrder = append(executionOrder, "second")
			return nil
		},
	}

	inspector3 := &mockInspector{
		name: "third",
		inspectFunc: func(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error {
			executionOrder = append(executionOrder, "third")
			return nil
		},
	}

	chain.AddInspector(inspector1)
	chain.AddInspector(inspector2)
	chain.AddInspector(inspector3)

	_, err := chain.Inspect(ctx, req, targetPath)

	if err != nil {
		t.Errorf("Inspect() should not error, got %v", err)
	}

	expectedOrder := []string{"first", "second", "third"}
	if len(executionOrder) != len(expectedOrder) {
		t.Fatalf("Execution order length = %d, want %d", len(executionOrder), len(expectedOrder))
	}

	for i, expected := range expectedOrder {
		if executionOrder[i] != expected {
			t.Errorf("Execution order[%d] = %v, want %v", i, executionOrder[i], expected)
		}
	}
}

type mockInspector struct {
	name        string
	called      bool
	inspectFunc func(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error
}

func (m *mockInspector) Name() string {
	return m.name
}

func (m *mockInspector) Inspect(ctx context.Context, r *http.Request, profile *domain.RequestProfile) error {
	m.called = true
	if m.inspectFunc != nil {
		return m.inspectFunc(ctx, r, profile)
	}
	return nil
}

var _ ports.RequestInspector = (*mockInspector)(nil)
