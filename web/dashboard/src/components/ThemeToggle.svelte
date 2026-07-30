<script>
  import { theme } from '../lib/stores/theme.svelte.js';

  // Three permanent states, one button each: system (follow the OS preference),
  // light, dark. Replacing the old single-toggle-plus-conditional-"auto"-reset
  // because that hid the mode you wanted behind a toggle-then-reset dance -
  // every mode is now one click and always visible. The store already owns the
  // mode, persistence and OS-preference reactivity.
  const options = [
    { mode: 'auto', icon: '◐', label: 'System theme (follow OS preference)' },
    { mode: 'light', icon: '☀', label: 'Light theme' },
    { mode: 'dark', icon: '☾', label: 'Dark theme' },
  ];
</script>

<div class="theme-toggle-group" role="group" aria-label="Colour theme">
  {#each options as opt (opt.mode)}
    <button
      class="theme-toggle icon-only"
      type="button"
      aria-pressed={theme.mode === opt.mode ? 'true' : 'false'}
      aria-label={opt.label}
      title={opt.label}
      onclick={() => theme.set(opt.mode)}
    >
      <span aria-hidden="true">{opt.icon}</span>
    </button>
  {/each}
</div>
