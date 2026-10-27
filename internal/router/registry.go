package router

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/pterm/pterm"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/logger"
)

// RejectFunc writes a security rejection (rate or size limit) in the wire format a
// route's clients expect. Headers such as Retry-After are already set when it runs.
type RejectFunc = func(w http.ResponseWriter, status int, message string)

type RouteInfo struct {
	Handler     http.HandlerFunc
	Reject      RejectFunc
	Description string
	Method      string
	Order       int
	IsProxy     bool
	// IsTranslator marks an inference route served by an API translator. It gets
	// the same rate and size limits as proxy routes, with rejections written by Reject.
	IsTranslator bool
}

type RouteRegistry struct {
	routes   map[string]RouteInfo
	logger   logger.StyledLogger
	orderSeq int
}

func NewRouteRegistry(logger logger.StyledLogger) *RouteRegistry {
	return &RouteRegistry{
		routes:   make(map[string]RouteInfo),
		logger:   logger,
		orderSeq: 0,
	}
}

func (r *RouteRegistry) Register(route string, handler http.HandlerFunc, description string) {
	r.RegisterWithMethod(route, handler, description, "GET")
}

func (r *RouteRegistry) RegisterWithMethod(route string, handler http.HandlerFunc, description, method string) {
	r.registerWithMethod(route, handler, description, method, false)
}

func (r *RouteRegistry) RegisterProxyRoute(route string, handler http.HandlerFunc, description string, method string) {
	wrappedHandler := func(w http.ResponseWriter, req *http.Request) {
		ctx := context.WithValue(req.Context(), constants.ContextRoutePrefixKey, route)
		handler(w, req.WithContext(ctx))
	}
	r.registerWithMethod(route, wrappedHandler, description, method, true)
}

// RegisterTranslatorRoute registers a route served by an API translator (for example
// /olla/anthropic/v1/messages). These are inference entry points, so they sit behind
// the same rate and size limits as proxy routes. reject lets the translator answer a
// rejection in its own error schema; nil falls back to plain text.
func (r *RouteRegistry) RegisterTranslatorRoute(route string, handler http.HandlerFunc, description, method string, reject RejectFunc) {
	r.registerWithMethod(route, handler, description, method, false)
	info := r.routes[route]
	info.IsTranslator = true
	info.Reject = reject
	r.routes[route] = info
}

func (r *RouteRegistry) registerWithMethod(route string, handler http.HandlerFunc, description, method string, isProxy bool) {
	r.routes[route] = RouteInfo{
		Handler:     handler,
		Description: description,
		Method:      method,
		Order:       r.orderSeq,
		IsProxy:     isProxy,
	}
	r.orderSeq++
}

func (r *RouteRegistry) WireUp(mux *http.ServeMux) {
	for route, info := range r.routes {
		mux.HandleFunc(route, info.Handler)
	}
	r.logRoutesTable()
}

func (r *RouteRegistry) logRoutesTable() {
	if len(r.routes) == 0 {
		return
	}

	type routeEntry struct {
		path   string
		method string
		desc   string
		order  int
	}

	var entries []routeEntry
	for route, info := range r.routes {
		entries = append(entries, routeEntry{
			path:   route,
			method: info.Method,
			desc:   info.Description,
			order:  info.Order,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].order < entries[j].order
	})

	tableData := [][]string{
		{"ROUTE", "METHOD", "DESCRIPTION"},
	}

	for _, entry := range entries {
		tableData = append(tableData, []string{
			entry.path,
			entry.method,
			entry.desc,
		})
	}

	r.logger.InfoWithCount("Registered web routes", len(entries))
	tableString, _ := pterm.DefaultTable.WithHasHeader().WithData(tableData).Srender()
	fmt.Print(tableString)
}

func (r *RouteRegistry) GetRoutes() map[string]RouteInfo {
	return r.routes
}

func (r *RouteRegistry) WireUpWithSecurityChain(mux *http.ServeMux, securityAdapters interface{}) {
	type securityAdapterProvider interface {
		CreateChainMiddleware() func(http.Handler) http.Handler
		CreateRateLimitMiddleware() func(http.Handler) http.Handler
	}

	// Optional: lets translator routes get the proxy chain with rejections written
	// in the translator's own error format.
	type rejectChainProvider interface {
		CreateChainMiddlewareWithReject(reject RejectFunc) func(http.Handler) http.Handler
	}

	// Optional: size validation for non-proxy routes. The full security chain
	// (proxy routes) already includes size enforcement; this ensures non-proxy
	// routes (status, health, stats) are also protected against oversized bodies.
	// Rate limiting on non-proxy routes is intentionally omitted: these endpoints
	// are internal/observability-only and applying a per-IP token bucket would
	// complicate operations without meaningful security benefit. Translator routes
	// are inference entry points, not observability, so they take the proxy chain.
	type sizeMiddlewareProvider interface {
		CreateSizeMiddleware() func(http.Handler) http.Handler
	}

	adapters, hasAdapters := securityAdapters.(securityAdapterProvider)

	if !hasAdapters {
		r.WireUp(mux)
		return
	}

	sizeProvider, hasSizeMiddleware := securityAdapters.(sizeMiddlewareProvider)

	for route, info := range r.routes {
		var handler http.Handler = info.Handler

		if info.IsProxy || info.IsTranslator {
			chain := adapters.CreateChainMiddleware()
			if rc, ok := securityAdapters.(rejectChainProvider); ok && info.IsTranslator {
				chain = rc.CreateChainMiddlewareWithReject(info.Reject)
			}
			handler = chain(handler)
			mux.Handle(route, handler)
		} else {
			// Non-proxy routes (status, health, stats) get logging via
			// CreateRateLimitMiddleware. Size validation is added on top when the
			// adapter supports it, running before the rate-limit/logging wrap so
			// oversized requests are rejected before consuming any quota or log budget.
			// Rate limiting itself is intentionally not enforced on these internal
			// endpoints; they are operational/observability-only and applying a
			// per-IP token bucket would complicate deployments without meaningful
			// security gain.
			handler = adapters.CreateRateLimitMiddleware()(handler)
			if hasSizeMiddleware {
				handler = sizeProvider.CreateSizeMiddleware()(handler)
			}
			mux.Handle(route, handler)
		}
	}
	r.logRoutesTable()
}
