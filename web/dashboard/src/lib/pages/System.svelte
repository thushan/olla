<script>
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  
  const processStats = $derived(dashboardStore.processStats || {});
  const status = $derived(dashboardStore.status);
  
  // Extract memory stats
  const memory = $derived(processStats.memory || {});
  const gc = $derived(processStats.garbage_collection || {});
  const goroutines = $derived(processStats.goroutines || {});
  const runtime = $derived(processStats.runtime || {});
  const allocations = $derived(processStats.allocations || {});
  
  // Get memory pressure color
  function getMemoryPressureColor(pressure) {
    switch(pressure?.toLowerCase()) {
      case 'low':
        return 'text-green-600 dark:text-green-400';
      case 'moderate':
        return 'text-yellow-600 dark:text-yellow-400';
      case 'high':
        return 'text-red-600 dark:text-red-400';
      default:
        return 'text-gray-600 dark:text-gray-400';
    }
  }
  
  // Get goroutine health color
  function getGoroutineHealthColor(health) {
    if (health?.toLowerCase().includes('healthy')) return 'text-green-600 dark:text-green-400';
    if (health?.toLowerCase().includes('warning')) return 'text-yellow-600 dark:text-yellow-400';
    return 'text-red-600 dark:text-red-400';
  }
</script>

<div class="space-y-6">
  <!-- Page Header -->
  <div>
    <h2 class="text-2xl font-bold text-gray-900 dark:text-white">System</h2>
    <p class="text-gray-600 dark:text-gray-400 mt-1">Process and runtime statistics</p>
  </div>
  
  <!-- Runtime Info -->
  <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
    <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Runtime Information</h3>
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
      <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
        <div class="text-sm text-gray-600 dark:text-gray-400">Uptime</div>
        <div class="text-xl font-semibold text-gray-900 dark:text-white mt-1">{runtime.uptime || 'Unknown'}</div>
      </div>
      <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
        <div class="text-sm text-gray-600 dark:text-gray-400">Go Version</div>
        <div class="text-xl font-semibold text-gray-900 dark:text-white mt-1">{runtime.go_version || 'Unknown'}</div>
      </div>
      <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
        <div class="text-sm text-gray-600 dark:text-gray-400">CPUs</div>
        <div class="text-xl font-semibold text-gray-900 dark:text-white mt-1">{runtime.num_cpu || 0}</div>
      </div>
      <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
        <div class="text-sm text-gray-600 dark:text-gray-400">GOMAXPROCS</div>
        <div class="text-xl font-semibold text-gray-900 dark:text-white mt-1">{runtime.gomaxprocs || 0}</div>
      </div>
    </div>
  </div>
  
  <!-- Memory Usage -->
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
    <!-- Memory Stats -->
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Memory Usage</h3>
      
      <div class="space-y-4">
        <div>
          <div class="flex justify-between items-center mb-2">
            <span class="text-sm text-gray-600 dark:text-gray-400">Heap Allocated</span>
            <span class="font-mono text-sm text-gray-900 dark:text-white">{memory.heap_alloc || '0 B'}</span>
          </div>
          <div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
            <div class="bg-blue-500 h-2 rounded-full" style="width: 40%"></div>
          </div>
        </div>
        
        <div>
          <div class="flex justify-between items-center mb-2">
            <span class="text-sm text-gray-600 dark:text-gray-400">Heap System</span>
            <span class="font-mono text-sm text-gray-900 dark:text-white">{memory.heap_sys || '0 B'}</span>
          </div>
          <div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
            <div class="bg-purple-500 h-2 rounded-full" style="width: 60%"></div>
          </div>
        </div>
        
        <div>
          <div class="flex justify-between items-center mb-2">
            <span class="text-sm text-gray-600 dark:text-gray-400">Stack In Use</span>
            <span class="font-mono text-sm text-gray-900 dark:text-white">{memory.stack_inuse || '0 B'}</span>
          </div>
          <div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
            <div class="bg-green-500 h-2 rounded-full" style="width: 20%"></div>
          </div>
        </div>
        
        <div class="pt-2 border-t border-gray-200 dark:border-gray-700">
          <div class="flex justify-between items-center">
            <span class="text-sm text-gray-600 dark:text-gray-400">Memory Pressure</span>
            <span class="font-medium {getMemoryPressureColor(memory.memory_pressure)}">{memory.memory_pressure || 'Unknown'}</span>
          </div>
        </div>
      </div>
    </div>
    
    <!-- GC Stats -->
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Garbage Collection</h3>
      
      <div class="space-y-4">
        <div class="flex justify-between items-center p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
          <span class="text-sm text-gray-600 dark:text-gray-400">Last GC</span>
          <span class="font-mono text-sm text-gray-900 dark:text-white">{gc.last_gc || 'Never'}</span>
        </div>
        
        <div class="flex justify-between items-center p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
          <span class="text-sm text-gray-600 dark:text-gray-400">Total GC Time</span>
          <span class="font-mono text-sm text-gray-900 dark:text-white">{gc.total_gc_time || '0s'}</span>
        </div>
        
        <div class="flex justify-between items-center p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
          <span class="text-sm text-gray-600 dark:text-gray-400">GC CPU Fraction</span>
          <span class="font-mono text-sm text-gray-900 dark:text-white">{((gc.gc_cpu_fraction || 0) * 100).toFixed(2)}%</span>
        </div>
        
        <div class="flex justify-between items-center p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
          <span class="text-sm text-gray-600 dark:text-gray-400">GC Cycles</span>
          <span class="font-mono text-sm text-gray-900 dark:text-white">{gc.num_gc_cycles || 0}</span>
        </div>
      </div>
    </div>
  </div>
  
  <!-- Goroutines and Allocations -->
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
    <!-- Goroutines -->
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Goroutines</h3>
      
      <div class="text-center py-6">
        <div class="text-5xl font-bold text-gray-900 dark:text-white">{goroutines.count || 0}</div>
        <div class="text-sm text-gray-600 dark:text-gray-400 mt-2">Active Goroutines</div>
        <div class="mt-4">
          <span class="font-medium {getGoroutineHealthColor(goroutines.health_status)}">{goroutines.health_status || 'Unknown'}</span>
        </div>
        {#if goroutines.cgo_calls}
          <div class="mt-4 text-sm text-gray-600 dark:text-gray-400">
            CGO Calls: {goroutines.cgo_calls}
          </div>
        {/if}
      </div>
    </div>
    
    <!-- Allocations -->
    <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Memory Allocations</h3>
      
      <div class="space-y-4">
        <div class="flex justify-between items-center p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
          <span class="text-sm text-gray-600 dark:text-gray-400">Total Mallocs</span>
          <span class="font-mono text-sm text-gray-900 dark:text-white">{(allocations.total_mallocs || 0).toLocaleString()}</span>
        </div>
        
        <div class="flex justify-between items-center p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
          <span class="text-sm text-gray-600 dark:text-gray-400">Total Frees</span>
          <span class="font-mono text-sm text-gray-900 dark:text-white">{(allocations.total_frees || 0).toLocaleString()}</span>
        </div>
        
        <div class="flex justify-between items-center p-3 bg-green-50 dark:bg-green-900/20 rounded-lg">
          <span class="text-sm text-gray-600 dark:text-gray-400">Net Objects</span>
          <span class="font-mono text-sm font-semibold text-green-700 dark:text-green-300">{(allocations.net_objects || 0).toLocaleString()}</span>
        </div>
      </div>
    </div>
  </div>
  
  <!-- Additional Memory Details -->
  <div class="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
    <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Memory Details</h3>
    
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
      <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
        <div class="text-sm text-gray-600 dark:text-gray-400">Heap In Use</div>
        <div class="text-lg font-semibold text-gray-900 dark:text-white mt-1">{memory.heap_inuse || '0 B'}</div>
      </div>
      <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
        <div class="text-sm text-gray-600 dark:text-gray-400">Heap Released</div>
        <div class="text-lg font-semibold text-gray-900 dark:text-white mt-1">{memory.heap_released || '0 B'}</div>
      </div>
      <div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
        <div class="text-sm text-gray-600 dark:text-gray-400">Total Allocated</div>
        <div class="text-lg font-semibold text-gray-900 dark:text-white mt-1">{memory.total_alloc || '0 B'}</div>
      </div>
    </div>
  </div>
</div>