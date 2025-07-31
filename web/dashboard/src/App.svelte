<script>
  import { onMount, onDestroy } from 'svelte';
  import { setThemeStore } from '$lib/stores/theme.svelte.js';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import { websocketService } from '$lib/services/websocket.js';
  import ThemeToggle from '$lib/components/ThemeToggle.svelte';
  import HeroStatus from '$lib/components/HeroStatus.svelte';
  import EndpointHealthGrid from '$lib/components/EndpointHealthGrid.svelte';
  import LiveRequestStream from '$lib/components/LiveRequestStream.svelte';
  import ModelUniverse from '$lib/components/ModelUniverse.svelte';
  import SecurityCommandCenter from '$lib/components/SecurityCommandCenter.svelte';
  import PerformanceAnalytics from '$lib/components/PerformanceAnalytics.svelte';
  import ConfigurationStatus from '$lib/components/ConfigurationStatus.svelte';
  import './app.css';
  
  // Initialize theme store
  const theme = setThemeStore();
  
  // Initialize dashboard and WebSocket on mount
  onMount(() => {
    dashboardStore.init();
    websocketService.connect();
  });
  
  onDestroy(() => {
    dashboardStore.destroy();
    websocketService.disconnect();
  });
</script>

<div class="min-h-screen bg-gray-50 dark:bg-gray-900 transition-colors duration-200">
  <!-- Header -->
  <header class="sticky top-0 z-50 w-full border-b bg-white/80 dark:bg-gray-900/80 backdrop-blur-md border-gray-200 dark:border-gray-800">
    <div class="container mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex h-16 items-center justify-between">
        <!-- Brand -->
        <div class="flex items-center gap-3">
          <div class="flex items-center gap-2">
            <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center">
              <span class="text-white font-bold text-sm">O</span>
            </div>
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">
              Olla Dashboard
            </h1>
          </div>
          <span class="hidden sm:inline-flex items-center rounded-full bg-blue-50 dark:bg-blue-900/20 px-3 py-1 text-xs font-medium text-blue-700 dark:text-blue-300">
            AI Infrastructure Monitor
          </span>
        </div>
        
        <!-- Actions -->
        <div class="flex items-center gap-4">
          <ThemeToggle />
        </div>
      </div>
    </div>
  </header>
  
  <!-- Main Content -->
  <main class="container mx-auto px-4 sm:px-6 lg:px-8 py-6">
    <!-- Hero Status -->
    <div class="mb-6">
      <HeroStatus />
    </div>
    
    <!-- Primary Grid -->
    <div class="grid grid-cols-1 xl:grid-cols-3 gap-6 mb-6">
      <!-- Main Column (2/3 width) -->
      <div class="xl:col-span-2 space-y-6">
        <!-- Endpoint Health Grid -->
        <EndpointHealthGrid />
        
        <!-- Performance Analytics -->
        <PerformanceAnalytics />
      </div>
      
      <!-- Side Column (1/3 width) -->
      <div class="space-y-6">
        <!-- Live Request Stream -->
        <LiveRequestStream />
        
        <!-- Security Command Center -->
        <SecurityCommandCenter />
      </div>
    </div>
    
    <!-- Secondary Grid -->
    <div class="grid grid-cols-1 xl:grid-cols-4 gap-6">
      <!-- Model Universe (3/4 width) -->
      <div class="xl:col-span-3">
        <ModelUniverse />
      </div>
      
      <!-- Configuration Status (1/4 width) -->
      <div>
        <ConfigurationStatus />
      </div>
    </div>
  </main>
  
  <!-- Footer -->
  <footer class="mt-12 border-t border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900">
    <div class="container mx-auto px-4 sm:px-6 lg:px-8 py-6">
      <div class="flex flex-col sm:flex-row items-center justify-between gap-4">
        <div class="flex items-center gap-4 text-sm text-gray-600 dark:text-gray-400">
          <span>Olla Dashboard</span>
          <span class="text-gray-400 dark:text-gray-600">•</span>
          <span>Built with Svelte 5 & TailwindCSS</span>
        </div>
        
        <div class="flex items-center gap-6 text-sm">
          <a 
            href="https://github.com/thushan/olla" 
            target="_blank" 
            rel="noopener noreferrer"
            class="text-gray-600 dark:text-gray-400 hover:text-blue-600 dark:hover:text-blue-400 transition-colors"
          >
            GitHub
          </a>
          <a 
            href="/api/docs" 
            class="text-gray-600 dark:text-gray-400 hover:text-blue-600 dark:hover:text-blue-400 transition-colors"
          >
            API Docs
          </a>
        </div>
      </div>
    </div>
  </footer>
</div>