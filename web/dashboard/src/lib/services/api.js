// API service for fetching data from Olla backend

// Use relative URLs when embedded in Olla, absolute URLs for standalone dev
const API_BASE = import.meta.env.VITE_API_BASE || 'http://localhost:40114';

class OllaAPI {
  constructor(baseURL = API_BASE) {
    this.baseURL = baseURL;
  }

  async fetch(endpoint, options = {}) {
    const url = `${this.baseURL}${endpoint}`;
    
    try {
      const response = await fetch(url, {
        ...options,
        headers: {
          'Content-Type': 'application/json',
          ...options.headers,
        },
      });

      if (!response.ok) {
        throw new Error(`API error: ${response.status} ${response.statusText}`);
      }

      return await response.json();
    } catch (error) {
      console.error(`Failed to fetch ${endpoint}:`, error);
      throw error;
    }
  }

  // System status
  async getStatus() {
    return this.fetch('/internal/status');
  }

  // Endpoint status
  async getEndpoints() {
    return this.fetch('/internal/status/endpoints');
  }

  // Model status
  async getModels() {
    return this.fetch('/internal/status/models');
  }

  // Model statistics
  async getModelStats() {
    return this.fetch('/internal/stats/models');
  }

  // Process statistics
  async getProcessStats() {
    return this.fetch('/internal/process');
  }

  // Unified models
  async getUnifiedModels(params = {}) {
    const queryString = new URLSearchParams(params).toString();
    return this.fetch(`/olla/models${queryString ? '?' + queryString : ''}`);
  }

  // Health check
  async getHealth() {
    return this.fetch('/internal/health');
  }

  // Version info
  async getVersion() {
    return this.fetch('/version');
  }
}

// Create singleton instance
export const api = new OllaAPI();

// WebSocket connection for real-time updates
export class OllaWebSocket {
  constructor(url = `ws://localhost:40114/ws/dashboard`) {
    this.url = url;
    this.ws = null;
    this.listeners = new Map();
    this.reconnectDelay = 1000;
    this.maxReconnectDelay = 30000;
    this.reconnectAttempts = 0;
  }

  connect() {
    try {
      this.ws = new WebSocket(this.url);
      
      this.ws.onopen = () => {
        console.log('WebSocket connected');
        this.reconnectDelay = 1000;
        this.reconnectAttempts = 0;
        this.emit('connected');
      };

      this.ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          this.emit('message', data);
          
          // Emit specific event types
          if (data.type) {
            this.emit(data.type, data);
          }
        } catch (error) {
          console.error('Failed to parse WebSocket message:', error);
        }
      };

      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        this.emit('error', error);
      };

      this.ws.onclose = () => {
        console.log('WebSocket disconnected');
        this.emit('disconnected');
        this.reconnect();
      };
    } catch (error) {
      console.error('Failed to create WebSocket:', error);
      this.reconnect();
    }
  }

  reconnect() {
    this.reconnectAttempts++;
    const delay = Math.min(this.reconnectDelay * this.reconnectAttempts, this.maxReconnectDelay);
    
    console.log(`Reconnecting in ${delay}ms...`);
    setTimeout(() => this.connect(), delay);
  }

  disconnect() {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  send(data) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data));
    } else {
      console.warn('WebSocket not connected');
    }
  }

  on(event, callback) {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, new Set());
    }
    this.listeners.get(event).add(callback);
  }

  off(event, callback) {
    if (this.listeners.has(event)) {
      this.listeners.get(event).delete(callback);
    }
  }

  emit(event, data) {
    if (this.listeners.has(event)) {
      this.listeners.get(event).forEach(callback => callback(data));
    }
  }
}

// Create WebSocket instance (will be connected when needed)
export const ws = new OllaWebSocket();