package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/thushan/olla/internal/adapter/proxy/core"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/pkg/format"
	"github.com/thushan/olla/pkg/nerdstats"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from any origin for dashboard
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Enable compression
	EnableCompression: true,
}

// WebSocketHub manages all WebSocket connections
type WebSocketHub struct {
	clients    map[*WebSocketClient]bool
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	broadcast  chan interface{}
	mu         sync.RWMutex
	app        *Application
	logger     logger.StyledLogger
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub(app *Application) *WebSocketHub {
	return &WebSocketHub{
		clients:    make(map[*WebSocketClient]bool),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
		broadcast:  make(chan interface{}, 256),
		app:        app,
		logger:     app.logger,
	}
}

// Run starts the WebSocket hub
func (h *WebSocketHub) Run(ctx context.Context) {
	h.logger.Info("WebSocket hub started and listening")
	
	// TODO: Subscribe to proxy events when available
	// For now, proxy events are not forwarded

	// Start periodic stats broadcasting
	statsTicker := time.NewTicker(2 * time.Second)
	defer statsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("WebSocket hub shutting down")
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Info("WebSocket client connected", "client", client.conn.RemoteAddr())

			// Send initial data
			go h.sendInitialData(client)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				h.logger.Info("WebSocket client disconnected", "client", client.conn.RemoteAddr())
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client's send channel is full, close it
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()

		// TODO: Enable when event subscription is available
		// case event, ok := <-eventSub:
		//	if ok {
		//		// Forward proxy events to clients
		//		h.broadcastProxyEvent(event)
		//	}

		case <-statsTicker.C:
			// Broadcast stats periodically
			h.broadcastStats()
		}
	}
}

// sendInitialData sends initial dashboard data to a newly connected client
func (h *WebSocketHub) sendInitialData(client *WebSocketClient) {
	// Send current status
	// TODO: Implement proper status retrieval
	status := h.getSystemStatus()
	if status != nil {
		client.send <- map[string]interface{}{
			"type":    "status",
			"payload": status,
		}
	}

	// Send endpoint health
	endpoints := h.getEndpointHealth()
	if endpoints != nil {
		client.send <- map[string]interface{}{
			"type":    "endpoint_health",
			"payload": endpoints,
		}
	}

	// Send system metrics
	metrics := h.getProcessMetrics()
	if metrics != nil {
		client.send <- map[string]interface{}{
			"type":    "system_metrics",
			"payload": metrics,
		}
	}
}

// broadcastStats broadcasts current statistics to all clients
func (h *WebSocketHub) broadcastStats() {
	if h.app.statsCollector != nil {
		stats := h.app.statsCollector.GetProxyStats()
		h.broadcast <- map[string]interface{}{
			"type":    "stats",
			"payload": stats,
		}
	}
}

// broadcastProxyEvent broadcasts a proxy event to all clients
func (h *WebSocketHub) broadcastProxyEvent(event core.ProxyEvent) {
	h.broadcast <- map[string]interface{}{
		"type": "proxy_event",
		"payload": map[string]interface{}{
			"timestamp":  event.Timestamp,
			"type":       string(event.Type),
			"endpoint":   event.Endpoint,
			"request_id": event.RequestID,
			"duration":   event.Duration.Milliseconds(),
			"error":      formatError(event.Error),
			"metadata":   event.Metadata,
		},
	}
}

// WebSocketClient represents a WebSocket client connection
type WebSocketClient struct {
	hub  *WebSocketHub
	conn *websocket.Conn
	send chan interface{}
}

// readPump pumps messages from the WebSocket connection to the hub
func (c *WebSocketClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.hub.logger.Error("WebSocket error", "error", err)
			}
			break
		}

		// Handle incoming messages (subscriptions, etc.)
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err == nil {
			c.handleMessage(msg)
		}
	}
}

// writePump pumps messages from the hub to the WebSocket connection
func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage handles incoming WebSocket messages
func (c *WebSocketClient) handleMessage(msg map[string]interface{}) {
	msgType, ok := msg["type"].(string)
	if !ok {
		return
	}

	switch msgType {
	case "subscribe":
		// Handle subscription requests
		// For now, all clients receive all updates
	case "ping":
		// Respond to ping
		c.send <- map[string]interface{}{
			"type": "pong",
		}
	}
}

// WebSocketHandler handles WebSocket upgrade requests
func WebSocketHandler(hub *WebSocketHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hub.logger.Info("WebSocket upgrade request", 
			"method", r.Method,
			"headers", r.Header,
			"remote", r.RemoteAddr,
		)
		
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			hub.logger.Error("Failed to upgrade connection", 
				"error", err,
				"headers", r.Header,
			)
			return
		}

		hub.logger.Info("WebSocket upgraded successfully", "remote", r.RemoteAddr)

		client := &WebSocketClient{
			hub:  hub,
			conn: conn,
			send: make(chan interface{}, 256),
		}

		client.hub.register <- client

		hub.logger.Info("Starting WebSocket pumps", "remote", r.RemoteAddr)
		go client.writePump()
		go client.readPump()
	}
}

// formatError safely formats an error for JSON serialization
func formatError(err error) interface{} {
	if err == nil {
		return nil
	}
	return err.Error()
}

// getSystemStatus retrieves the current system status
func (h *WebSocketHub) getSystemStatus() map[string]interface{} {
	// Build status similar to statusHandler
	ctx := context.Background()
	endpoints, err := h.app.repository.GetAll(ctx)
	if err != nil {
		h.logger.Error("Failed to get endpoints", "error", err)
		endpoints = nil
	}
	
	healthyCount := 0
	for _, ep := range endpoints {
		if ep.Status == "healthy" {
			healthyCount++
		}
	}
	
	proxyStats := h.app.statsCollector.GetProxyStats()
	
	return map[string]interface{}{
		"system": map[string]interface{}{
			"status":               "operational",
			"total_requests":       proxyStats.TotalRequests,
			"successful_requests":  proxyStats.SuccessfulRequests,
			"failed_requests":      proxyStats.FailedRequests,
			"avg_latency_ms":       proxyStats.AverageLatency,
			"active_connections":   0, // TODO: Track active connections
			"security_violations":  0, // TODO: Track security violations
		},
		"endpoints": endpoints,
		"total_endpoints":    len(endpoints),
		"healthy_endpoints":  healthyCount,
		"routable_endpoints": healthyCount,
	}
}

// getEndpointHealth retrieves endpoint health information
func (h *WebSocketHub) getEndpointHealth() []map[string]interface{} {
	ctx := context.Background()
	endpoints, err := h.app.repository.GetAll(ctx)
	if err != nil {
		h.logger.Error("Failed to get endpoints", "error", err)
		return nil
	}
	
	result := make([]map[string]interface{}, 0, len(endpoints))
	for _, ep := range endpoints {
		result = append(result, map[string]interface{}{
			"name":                  ep.Name,
			"url":                   ep.URL,
			"status":                ep.Status,
			"priority":              ep.Priority,
			"last_latency":          ep.LastLatency.String(),
			"consecutive_failures":  ep.ConsecutiveFailures,
		})
	}
	
	return result
}

// getProcessMetrics retrieves process metrics
func (h *WebSocketHub) getProcessMetrics() map[string]interface{} {
	// Similar to processStatsHandler but returns a map
	stats := nerdstats.Snapshot(h.app.StartTime)
	
	return map[string]interface{}{
		"timestamp": time.Now(),
		"memory": map[string]interface{}{
			"heap_alloc":       format.Bytes(stats.HeapAlloc),
			"heap_sys":         format.Bytes(stats.HeapSys),
			"heap_inuse":       format.Bytes(stats.HeapInuse),
			"heap_released":    format.Bytes(stats.HeapReleased),
			"stack_inuse":      format.Bytes(stats.StackInuse),
			"total_alloc":      format.Bytes(stats.TotalAlloc),
			"memory_pressure":  stats.GetMemoryPressure(),
		},
		"garbage_collection": map[string]interface{}{
			"last_gc":          stats.LastGC.Format(time.RFC3339),
			"total_gc_time":    format.Duration(stats.TotalGCTime),
			"gc_cpu_fraction":  stats.GCCPUFraction,
			"num_gc_cycles":    stats.NumGC,
		},
		"goroutines": map[string]interface{}{
			"health_status": stats.GetGoroutineHealthStatus(),
			"count":         stats.NumGoroutines,
			"cgo_calls":     stats.NumCgoCall,
		},
		"runtime": map[string]interface{}{
			"uptime":      format.Duration(stats.Uptime),
			"go_version":  stats.GoVersion,
			"num_cpu":     stats.NumCPU,
			"gomaxprocs":  stats.GOMAXPROCS,
		},
		"allocations": map[string]interface{}{
			"total_mallocs": stats.Mallocs,
			"total_frees":   stats.Frees,
			"net_objects":   int64(stats.Mallocs) - int64(stats.Frees),
		},
	}
}