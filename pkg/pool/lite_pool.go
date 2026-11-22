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
//
//	type RequestContext struct { ... }
//	func (r *RequestContext) Reset() { ... }
//
//	pool, err := NewLitePool(func() *RequestContext {
//	  return &RequestContext{}
//	})
//	if err != nil {
//	  // handle error
//	}
//
//	ctx, err := pool.Get()
//	if err != nil {
//	  // handle error
//	}
//	...
//	pool.Put(ctx)
//
// Note: This is intentionally minimal and inlined for performance-sensitive paths.
// If Go ever adds generics to sync.Pool (e.g. Go 1.23+), this becomes obsolete.

import (
	"errors"
	"reflect"
	"sync"
)

type Resettable interface {
	Reset()
}

type Pool[T any] struct {
	pool sync.Pool
	new  func() T
}

func NewLitePool[T any](newFn func() T) (*Pool[T], error) {
	if newFn == nil {
		return nil, errors.New("litepool: constructor must not be nil")
	}
	// Validate early that the result is non-nil
	test := newFn()
	if isNilValue(test) {
		return nil, errors.New("litepool: constructor returned nil")
	}

	return &Pool[T]{
		pool: sync.Pool{
			New: func() any {
				return newFn()
			},
		},
		new: newFn,
	}, nil
}

func (p *Pool[T]) Get() (T, error) {
	raw := p.pool.Get()
	// sync.Pool.Get() returns nil when New is nil and the pool is empty.
	// Guard here before the type assertion so interface-typed pools (Pool[any])
	// never panic on an untyped nil returned by a stateful factory.
	if raw == nil {
		var zero T
		return zero, errors.New("litepool: factory returned nil")
	}
	v, ok := raw.(T)
	if !ok {
		var zero T
		return zero, errors.New("litepool: pool returned unexpected type")
	}
	// A stateful factory can violate its contract after construction by returning
	// a typed nil (e.g. (*T)(nil)). Return an error rather than panicking so
	// callers can propagate it cleanly without crashing in-flight requests.
	if isNilValue(v) {
		var zero T
		return zero, errors.New("litepool: factory returned nil")
	}
	return v, nil
}

// isNilValue reports whether v is nil, handling both untyped nil (interface{} == nil)
// and typed nils (e.g. (*T)(nil) stored in an interface). The reflect path is only
// reached for nilable kinds; value types like structs always return false.
func isNilValue[T any](v T) bool {
	if any(v) == nil {
		return true
	}
	rv := reflect.ValueOf(any(v))
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}

func (p *Pool[T]) Put(v T) {
	if r, ok := any(v).(Resettable); ok {
		r.Reset()
	}
	p.pool.Put(v)
}
