<script>
  import { getThemeStore } from '$lib/stores/theme.svelte.js';
  
  const theme = getThemeStore();
  
  // Icons for sun and moon
  const sunIcon = `<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" class="w-5 h-5">
    <path stroke-linecap="round" stroke-linejoin="round" d="M12 3v2.25m6.364.386l-1.591 1.591M21 12h-2.25m-.386 6.364l-1.591-1.591M12 18.75V21m-4.773-4.227l-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z" />
  </svg>`;
  
  const moonIcon = `<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" class="w-5 h-5">
    <path stroke-linecap="round" stroke-linejoin="round" d="M21.752 15.002A9.718 9.718 0 0118 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 003 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 009.002-5.998z" />
  </svg>`;
</script>

<button
  onclick={() => theme.toggle()}
  class="theme-toggle"
  aria-label="Toggle theme"
  title={theme.isDark ? 'Switch to light mode' : 'Switch to dark mode'}
>
  <div class="icon-container">
    {#if theme.isDark}
      <div class="icon moon-icon" transition:scale>
        {@html moonIcon}
      </div>
    {:else}
      <div class="icon sun-icon" transition:scale>
        {@html sunIcon}
      </div>
    {/if}
  </div>
</button>

<style>
  .theme-toggle {
    position: relative;
    padding: 0.5rem;
    border-radius: 0.5rem;
    transition: all;
    transition-duration: 300ms;
    background-color: var(--bg-secondary);
    color: var(--text-primary);
    border: none;
    cursor: pointer;
  }
  
  .theme-toggle:hover {
    background-color: var(--bg-tertiary);
  }
  
  .theme-toggle:focus {
    outline: 2px solid var(--color-blue);
    outline-offset: 2px;
  }
  
  .icon-container {
    position: relative;
    width: 1.25rem;
    height: 1.25rem;
  }
  
  .icon {
    position: absolute;
    top: 0; right: 0; bottom: 0; left: 0;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  
  .sun-icon {
    color: var(--color-orange);
  }
  
  .moon-icon {
    color: var(--color-yellow);
  }
  
  /* Smooth transition between icons */
  :global(.icon-enter) {
    opacity: 0;
    transform: scale(0.8) rotate(-90deg);
  }
  
  :global(.icon-enter-active) {
    transition: all 0.3s ease-out;
  }
  
  :global(.icon-enter-to) {
    opacity: 1;
    transform: scale(1) rotate(0);
  }
  
  :global(.icon-exit) {
    opacity: 1;
    transform: scale(1) rotate(0);
  }
  
  :global(.icon-exit-active) {
    transition: all 0.3s ease-in;
  }
  
  :global(.icon-exit-to) {
    opacity: 0;
    transform: scale(0.8) rotate(90deg);
  }
</style>