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

<div class="app-container">
  <!-- Background pattern -->
  <div class="fixed inset-0 grid-pattern opacity-50"></div>
  
  <!-- Main content -->
  <div class="relative z-10">
    <!-- Header -->
    <header class="app-header">
      <div class="header-content">
        <div class="header-brand">
          <h1 class="app-title gradient-text">
            Olla Dashboard
          </h1>
          <span class="app-subtitle">
            AI Infrastructure Monitor
          </span>
        </div>
        
        <div class="header-actions">
          <ThemeToggle />
        </div>
      </div>
    </header>
    
    <!-- Main dashboard content -->
    <main class="dashboard-main">
      <div class="dashboard-layout">
        <!-- Main Content Area -->
        <div class="main-content">
          <!-- Hero Status Panel -->
          <section class="dashboard-section">
            <HeroStatus />
          </section>
          
          <!-- Two Column Grid -->
          <div class="grid-container">
            <!-- Left Column -->
            <div class="grid-column">
              <!-- Endpoint Health Grid -->
              <section class="dashboard-section">
                <EndpointHealthGrid />
              </section>
              
              <!-- Security Command Center -->
              <section class="dashboard-section">
                <SecurityCommandCenter />
              </section>
            </div>
            
            <!-- Right Column -->
            <div class="grid-column">
              <!-- Live Request Stream -->
              <section class="dashboard-section">
                <LiveRequestStream />
              </section>
              
              <!-- Performance Analytics -->
              <section class="dashboard-section">
                <PerformanceAnalytics />
              </section>
            </div>
          </div>
          
          <!-- Model Universe -->
          <section class="dashboard-section">
            <ModelUniverse />
          </section>
        </div>
        
        <!-- Sidebar -->
        <aside class="dashboard-sidebar">
          <ConfigurationStatus />
        </aside>
      </div>
    </main>
    
    <!-- Footer -->
    <footer class="app-footer">
      <div class="footer-content">
        <div class="footer-info">
          <span class="footer-text">Olla Dashboard</span>
          <span class="footer-separator">•</span>
          <span class="footer-text">Built with Svelte 5 & TailwindCSS</span>
        </div>
        
        <div class="footer-links">
          <a href="https://github.com/thushan/olla" target="_blank" rel="noopener" class="footer-link">
            GitHub
          </a>
          <span class="footer-separator">•</span>
          <a href="/api/docs" class="footer-link">
            API Docs
          </a>
        </div>
      </div>
    </footer>
  </div>
</div>

<style>
  .app-container {
    min-height: 100vh;
  }
  
  .app-header {
    position: sticky;
    top: 0;
    z-index: 50;
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    border-bottom: 1px solid;
    background-color: rgba(var(--bg-primary-rgb, 251, 251, 251), 0.8);
    border-color: var(--bg-tertiary);
  }
  
  .dark .app-header {
    background-color: rgba(var(--bg-primary-rgb, 1, 22, 39), 0.8);
  }
  
  .header-content {
    width: 100%;
    max-width: 1536px;
    margin: 0 auto;
    padding: 1rem 1.5rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  
  .header-brand {
    display: flex;
    align-items: baseline;
    gap: 0.75rem;
  }
  
  .app-title {
    font-size: 1.5rem;
    line-height: 2rem;
    font-weight: 700;
  }
  
  .gradient-text {
    background-clip: text;
    -webkit-background-clip: text;
    color: transparent;
    background-image: linear-gradient(to right, var(--color-blue), var(--color-purple), var(--color-cyan));
  }
  
  .app-subtitle {
    font-size: 0.875rem;
    line-height: 1.25rem;
    color: var(--text-secondary);
  }
  
  .header-actions {
    display: flex;
    align-items: center;
    gap: 1rem;
  }
  
  .dashboard-main {
    flex: 1 1 0%;
  }
  
  .dashboard-layout {
    display: flex;
    gap: 1.5rem;
    max-width: 1536px;
    margin: 0 auto;
    padding: 2rem 1.5rem;
  }
  
  .main-content {
    flex: 1 1 0%;
    min-width: 0;
  }
  
  .dashboard-sidebar {
    width: 20rem;
    flex-shrink: 0;
  }
  
  @media (max-width: 1280px) {
    .dashboard-layout {
      flex-direction: column;
    }
    
    .dashboard-sidebar {
      width: 100%;
    }
  }
  
  .dashboard-section {
    margin-bottom: 1.5rem;
  }
  
  .grid-container {
    display: grid;
    grid-template-columns: repeat(1, minmax(0, 1fr));
    gap: 1.5rem;
  }
  
  @media (min-width: 1280px) {
    .grid-container {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  
  .grid-column {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }
  
  .coming-soon-card {
    background-color: var(--bg-secondary);
    border-radius: 0.75rem;
    padding: 2rem;
    text-align: center;
    border: 2px dashed;
    opacity: 0.6;
    border-color: var(--bg-tertiary);
  }
  
  .coming-soon-icon {
    font-size: 2.25rem;
    line-height: 2.5rem;
    margin-bottom: 1rem;
  }
  
  .coming-soon-title {
    font-size: 1.125rem;
    line-height: 1.75rem;
    font-weight: 600;
    color: var(--text-primary);
    margin-bottom: 0.5rem;
  }
  
  .coming-soon-text {
    font-size: 0.875rem;
    line-height: 1.25rem;
    color: var(--text-secondary);
  }
  
  .app-footer {
    margin-top: auto;
    border-top: 1px solid;
    border-color: var(--bg-tertiary);
  }
  
  .footer-content {
    width: 100%;
    max-width: 1536px;
    margin: 0 auto;
    padding: 1.5rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
  }
  
  @media (min-width: 768px) {
    .footer-content {
      flex-direction: row;
    }
  }
  
  .footer-info, .footer-links {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.875rem;
    line-height: 1.25rem;
  }
  
  .footer-text {
    color: var(--text-secondary);
  }
  
  .footer-separator {
    color: var(--text-muted);
  }
  
  .footer-link {
    text-decoration: none;
    transition: all 200ms;
    color: var(--color-blue);
  }
  
  .footer-link:hover {
    text-decoration: underline;
  }
</style>