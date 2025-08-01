<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  // Get reactive debug info
  const connections = $derived(dashboardStore.connections);
  const status = $derived(dashboardStore.status);
  const currentMetrics = $derived(dashboardStore.currentMetrics);
  const connectionStatus = $derived.by(() => {
    try {
      return dashboardStore.getConnectionStatus();
    } catch (e) {
      return { error: e.message };
    }
  });
  
  // Debug logging
  $effect(() => {
    console.log('[DebugPanel] Connections:', connections.length);
    console.log('[DebugPanel] Status:', status?.system?.active_connections);
    console.log('[DebugPanel] CurrentMetrics:', currentMetrics);
    console.log('[DebugPanel] ConnectionStatus:', connectionStatus);
  });
</script>

<div class="fixed bottom-4 right-4 bg-black/80 text-white p-4 rounded-lg text-xs font-mono max-w-md">
  <div class="text-yellow-400 mb-2">🐛 Debug Panel</div>
  
  <div class="space-y-1">
    <div>Connections: <span class="text-green-400">{connections.length}</span></div>
    <div>Active Connections: <span class="text-blue-400">{status?.system?.active_connections || 0}</span></div>
    <div>Real Data: <span class="text-purple-400">{currentMetrics.hasRealData}</span></div>
    <div>Last Update: <span class="text-gray-400">{new Date(currentMetrics.lastUpdate).toLocaleTimeString()}</span></div>
    <div>Connection Status:</div>
    <div class="ml-2 text-gray-300">
      WS: {connectionStatus.websocket ? '✅' : '❌'}<br>
      Polling: {connectionStatus.polling ? '✅' : '❌'}<br>
      Started: {connectionStatus.isStarted ? '✅' : '❌'}
    </div>
  </div>
</div>