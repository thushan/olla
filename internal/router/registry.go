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
}

type RouteRegistry struct {
	routes   map[string]RouteInfo
	logger   *logger.StyledLogger
	orderSeq int
}

func NewRouteRegistry(logger *logger.StyledLogger) *RouteRegistry {
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
	r.routes[route] = RouteInfo{
		Handler:     handler,
		Description: description,
		Method:      method,
		Order:       r.orderSeq,
	}
	r.orderSeq++
}

func (r *RouteRegistry) RegisterProxyRoute(route string, handler http.HandlerFunc, description string, method string) {
	wrappedHandler := func(w http.ResponseWriter, req *http.Request) {
		ctx := context.WithValue(req.Context(), constants.ProxyPathPrefix, route)
		handler(w, req.WithContext(ctx))
	}
	r.RegisterWithMethod(route, wrappedHandler, description, method)
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

	// Sort routes by registration order
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

	// Build table
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

	r.logger.Info(fmt.Sprintf("Registered routes %s", pterm.Style{r.logger.Theme.Counts}.Sprintf("(%d)", len(entries))))
	tableString, _ := pterm.DefaultTable.WithHasHeader().WithData(tableData).Srender()
	fmt.Print(tableString)
}

func (r *RouteRegistry) GetRoutes() map[string]RouteInfo {
	return r.routes
}
