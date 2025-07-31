import { dashboardStore } from '../stores/dashboard.svelte.js';

export class WebSocketService {
  constructor() {
    this.ws = null;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 5;
    this.reconnectDelay = 1000;
    this.isIntentionallyClosed = false;
  }

  connect() {
    // Use the same host but with ws:// protocol
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    // If running on the dashboard path, we need to use the base host
    const host = window.location.host;
    const wsUrl = `${protocol}//${host}/internal/ws`;
    
    console.log('Connecting to WebSocket:', wsUrl);
    
    try {
      this.ws = new WebSocket(wsUrl);
      
      this.ws.onopen = () => {
        console.log('WebSocket connected');
        this.reconnectAttempts = 0;
        this.reconnectDelay = 1000;
        dashboardStore.setWebSocketConnected(true);
        
        // Send a ping to test connection
        this.send({ type: 'ping' });
        
        // Request initial data
        this.send({ type: 'subscribe', topics: ['stats', 'events', 'health'] });
      };
      
      this.ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          this.handleMessage(data);
        } catch (err) {
          console.error('Failed to parse WebSocket message:', err);
        }
      };
      
      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
      };
      
      this.ws.onclose = () => {
        console.log('WebSocket disconnected');
        dashboardStore.setWebSocketConnected(false);
        
        if (!this.isIntentionallyClosed && this.reconnectAttempts < this.maxReconnectAttempts) {
          this.reconnectAttempts++;
          const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);
          console.log(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
          
          setTimeout(() => {
            this.connect();
          }, delay);
        }
      };
    } catch (err) {
      console.error('Failed to create WebSocket:', err);
    }
  }
  
  handleMessage(data) {
    console.log('WebSocket message received:', data.type, data);
    
    switch (data.type) {
      case 'stats':
        dashboardStore.updateStats(data.payload);
        break;
        
      case 'endpoint_health':
        dashboardStore.updateEndpointHealth(data.payload);
        break;
        
      case 'proxy_event':
        dashboardStore.addEvent(data.payload);
        break;
        
      case 'system_metrics':
        dashboardStore.updateSystemMetrics(data.payload);
        break;
        
      case 'status':
        // Handle initial status update
        if (data.payload) {
          dashboardStore.updateStatus(data.payload);
        }
        break;
        
      case 'pong':
        console.log('Received pong from server');
        break;
        
      default:
        console.warn('Unknown WebSocket message type:', data.type);
    }
  }
  
  send(data) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data));
    } else {
      console.warn('WebSocket not connected, cannot send:', data);
    }
  }
  
  disconnect() {
    this.isIntentionallyClosed = true;
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
  
  isConnected() {
    return this.ws && this.ws.readyState === WebSocket.OPEN;
  }
}

// Create singleton instance
export const websocketService = new WebSocketService();