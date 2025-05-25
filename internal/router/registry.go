package router

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/theme"
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

	// Collect routes in registration order
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

	// Sort by registration order
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].order < entries[j].order
	})

	// Create table using simple formatting instead of pterm
	appTheme := theme.GetTheme("default")
	countStyle := appTheme.CountsStyle()

	r.logger.Info(fmt.Sprintf("Registered routes %s", countStyle.Render(fmt.Sprintf("(%d)", len(entries)))))

	// Build table manually
	fmt.Printf("%-25s %-10s %s\n", "ROUTE", "METHOD", "DESCRIPTION")
	fmt.Println(strings.Repeat("─", 70))

	for _, entry := range entries {
		fmt.Printf("%-25s %-10s %s\n", entry.path, entry.method, entry.desc)
	}
	fmt.Println()
}

func (r *RouteRegistry) GetRoutes() map[string]RouteInfo {
	return r.routes
}