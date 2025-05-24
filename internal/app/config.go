package app

import "github.com/thushan/olla/internal/config"

// setConfig - minimal thread safety addition
func (a *Application) setConfig(cfg *config.Config) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	a.config = cfg
}

// getConfig - helper for safe access
func (a *Application) getConfig() *config.Config {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	return a.config
}
