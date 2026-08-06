package app

import (
	"time"
)

const (
	DefaultConnectionTimeout   = 30 * time.Second
	DefaultConnectionKeepAlive = 30 * time.Second
	DefaultResponseTimeout     = 600 * time.Second
	DefaultReadTimeout         = 300 * time.Second
	DefaultLoadBalancer        = "priority"
	DefaultStreamBufferSize    = 8 * 1024 // 8KB

	// DefaultReadHeaderTimeout guards the inbound server against Slowloris-style
	// attacks where a client opens a connection and trickles headers indefinitely.
	// 10 s is enough for any legitimate client to send its headers; backends that
	// are slow to respond are covered by ConnectionTimeout instead.
	DefaultReadHeaderTimeout = 10 * time.Second
)
