<script>
  import { onMount, onDestroy } from 'svelte';
  import { setThemeStore } from '$lib/stores/theme.svelte.js';
  import { dashboardStore } from '$lib/stores/dashboard.svelte.js';
  import { websocketService } from '$lib/services/websocket.js';
  import ThemeToggle from '$lib/components/ThemeToggle.svelte';
  import HeroMetrics from '$lib/components/HeroMetrics.svelte';
  import EndpointMap from '$lib/components/EndpointMap.svelte';
  import RequestFlow from '$lib/components/RequestFlow.svelte';
  import ModelGalaxy from '$lib/components/ModelGalaxy.svelte';
  import PerformanceMetrics from '$lib/components/PerformanceMetrics.svelte';
  import SecurityRadar from '$lib/components/SecurityRadar.svelte';
  import SystemHealth from '$lib/components/SystemHealth.svelte';
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
  
  // Get current time for greeting
  const hour = new Date().getHours();
  const greeting = hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening';
</script>

<div class="min-h-screen bg-gradient-to-br from-gray-50 via-gray-50 to-blue-50 dark:from-gray-900 dark:via-gray-900 dark:to-blue-950 transition-all duration-500">
  <!-- Animated background patterns -->
  <div class="fixed inset-0 overflow-hidden pointer-events-none">
    <div class="absolute -top-40 -right-40 w-80 h-80 bg-purple-300 dark:bg-purple-900 rounded-full mix-blend-multiply dark:mix-blend-screen filter blur-xl opacity-20 animate-blob"></div>
    <div class="absolute -bottom-40 -left-40 w-80 h-80 bg-blue-300 dark:bg-blue-900 rounded-full mix-blend-multiply dark:mix-blend-screen filter blur-xl opacity-20 animate-blob animation-delay-2000"></div>
    <div class="absolute top-1/2 left-1/2 transform -translate-x-1/2 -translate-y-1/2 w-80 h-80 bg-pink-300 dark:bg-pink-900 rounded-full mix-blend-multiply dark:mix-blend-screen filter blur-xl opacity-20 animate-blob animation-delay-4000"></div>
  </div>

  <!-- Main Layout -->
  <div class="relative z-10">
    <!-- Header -->
    <header class="border-b border-gray-200/50 dark:border-gray-800/50 backdrop-blur-xl bg-white/30 dark:bg-gray-900/30">
      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div class="flex items-center justify-between h-16">
          <!-- Logo & Title -->
          <div class="flex items-center gap-4">
            <div class="relative">
              <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center shadow-lg shadow-purple-500/30">
                <span class="text-white font-bold text-lg">O</span>
              </div>
              <div class="absolute -bottom-1 -right-1 w-3 h-3 bg-green-500 rounded-full border-2 border-white dark:border-gray-900 animate-pulse"></div>
            </div>
            <div>
              <h1 class="text-xl font-bold bg-gradient-to-r from-gray-900 to-gray-600 dark:from-white dark:to-gray-300 bg-clip-text text-transparent">
                Olla Dashboard
              </h1>
              <p class="text-xs text-gray-600 dark:text-gray-400">{greeting} • Real-time AI Infrastructure Monitor</p>
            </div>
          </div>
          
          <!-- Actions -->
          <div class="flex items-center gap-4">
            <div class="hidden sm:flex items-center gap-2 px-3 py-1.5 rounded-lg bg-gray-100/50 dark:bg-gray-800/50 backdrop-blur-sm">
              <div class="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
              <span class="text-xs font-medium text-gray-700 dark:text-gray-300">Live</span>
            </div>
            <ThemeToggle />
          </div>
        </div>
      </div>
    </header>

    <!-- Main Content -->
    <main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <!-- Hero Metrics -->
      <div class="mb-8">
        <HeroMetrics />
      </div>
      
      <!-- Primary Dashboard Grid -->
      <div class="grid grid-cols-1 lg:grid-cols-12 gap-6 mb-8">
        <!-- System Health Overview (spans 4 columns) -->
        <div class="lg:col-span-4">
          <SystemHealth />
        </div>
        
        <!-- Endpoint Map (spans 8 columns) -->
        <div class="lg:col-span-8">
          <EndpointMap />
        </div>
      </div>
      
      <!-- Secondary Grid -->
      <div class="grid grid-cols-1 lg:grid-cols-12 gap-6 mb-8">
        <!-- Request Flow (spans 7 columns) -->
        <div class="lg:col-span-7">
          <RequestFlow />
        </div>
        
        <!-- Security Radar (spans 5 columns) -->
        <div class="lg:col-span-5">
          <SecurityRadar />
        </div>
      </div>
      
      <!-- Performance Section -->
      <div class="mb-8">
        <PerformanceMetrics />
      </div>
      
      <!-- Model Galaxy -->
      <div>
        <ModelGalaxy />
      </div>
    </main>
  </div>
</div>

<style>
  @keyframes blob {
    0% {
      transform: translate(0px, 0px) scale(1);
    }
    33% {
      transform: translate(30px, -50px) scale(1.1);
    }
    66% {
      transform: translate(-20px, 20px) scale(0.9);
    }
    100% {
      transform: translate(0px, 0px) scale(1);
    }
  }
  
  .animate-blob {
    animation: blob 7s infinite;
  }
  
  .animation-delay-2000 {
    animation-delay: 2s;
  }
  
  .animation-delay-4000 {
    animation-delay: 4s;
  }
</style>