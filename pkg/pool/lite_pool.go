package pool

// Pool is a strongly typed wrapper around sync.Pool with optional Reset() support.
// It eliminates the need for unsafe type assertions (interface{} casts) and plays
// nicely with golangci-lint. Objects returned from Get() are guaranteed to be the
// correct type. If the pooled type implements the Resettable interface, it will be
// automatically zeroed before being returned to the pool via Put().
//
// Designed for internal use where the constructor guarantees type safety, so the
// type assertion in Get() is safe and explicitly silenced.
//
// Example:
//   type RequestContext struct { ... }
//   func (r *RequestContext) Reset() { ... }
//
//   pool := NewLitePool(func() *RequestContext {
//     return &RequestContext{}
//   })
//
//   ctx := pool.Get()
//   ...
//   pool.Put(ctx)
//
// Note: This is intentionally minimal and inlined for performance-sensitive paths.
// If Go ever adds generics to sync.Pool (e.g. Go 1.23+), this becomes obsolete.

import "sync"

type Resettable interface {
	Reset()
}

type Pool[T any] struct {
	pool sync.Pool
	new  func() T
}

func NewLitePool[T any](newFn func() T) *Pool[T] {
	if newFn == nil {
		panic("litepool: constructor must not be nil")
	}
	// Validate early that the result is non-nil
	test := newFn()
	if any(test) == nil {
		panic("litepool: constructor returned nil")
	}

	return &Pool[T]{
		pool: sync.Pool{
			New: func() any {
				v := newFn()
				if any(v) == nil {
					panic("litepool: constructor returned nil")
				}
				return v
			},
		},
		new: newFn,
	}
}

func (p *Pool[T]) Get() T {
	//nolint:forcetypeassert // safe due to validated New
	return p.pool.Get().(T)
}

func (p *Pool[T]) Put(v T) {
	if r, ok := any(v).(Resettable); ok {
		r.Reset()
	}
	p.pool.Put(v)
}
