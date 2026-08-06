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

type RouteInfo struct {
	Handler     http.HandlerFunc
	Description string
	Method      string
	Order       int
	IsProxy     bool
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

	// Optional: size validation for non-proxy routes. The full security chain
	// (proxy routes) already includes size enforcement; this ensures non-proxy
	// routes (status, health, stats) are also protected against oversized bodies.
	// Rate limiting on non-proxy routes is intentionally omitted: these endpoints
	// are internal/observability-only and applying a per-IP token bucket would
	// complicate operations without meaningful security benefit.
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

		if info.IsProxy {
			handler = adapters.CreateChainMiddleware()(handler)
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
