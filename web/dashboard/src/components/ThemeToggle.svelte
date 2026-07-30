<script>
  import { theme } from '../lib/stores/theme.svelte.js';

  // Auto / light / dark. aria-pressed tracks whether a manual override is active.
  const icon = $derived(
    theme.mode === 'auto' ? '◐' : theme.mode === 'dark' ? '●' : '○'
  );
  const pressed = $derived(theme.mode !== 'auto');

  // Every click must visibly change the theme, so the announced action is
  // always "switch to the opposite of what's currently rendered" rather than
  // a fixed next-in-cycle name that might already match the system.
  const opposite = $derived(theme.resolved === 'dark' ? 'light' : 'dark');
  const stateDescription = $derived(
    theme.mode === 'auto'
      ? `Theme: auto, following system, currently ${theme.resolved}.`
      : `Theme: ${theme.mode}.`
  );
  const toggleLabel = $derived(`${stateDescription} Switch to ${opposite}.`);
</script>

<div class="theme-toggle-group">
  <button
    class="theme-toggle icon-only"
    type="button"
    aria-pressed={pressed ? 'true' : 'false'}
    aria-label={toggleLabel}
    title={toggleLabel}
    onclick={() => theme.toggle()}
  >
    <span aria-hidden="true">{icon}</span>
  </button>
  {#if theme.mode !== 'auto'}
    <button
      class="theme-auto-reset"
      type="button"
      aria-label="Reset theme to follow system (auto)"
      title="Reset theme to follow system (auto)"
      onclick={() => theme.set('auto')}
    >
      auto
    </button>
  {/if}
</div>
