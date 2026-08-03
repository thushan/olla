<script>
  import { theme } from '../lib/stores/theme.svelte.ts';

  // One icon-only button that cycles auto -> light -> dark -> auto. The
  // glyph reflects `mode` (not `resolved`) so each step is a visible change
  // even when the resolved theme happens to match the next mode. The store
  // owns the mode, persistence and OS-preference reactivity; this component
  // only renders the current state and forwards clicks to cycle().
  const byMode = {
    auto: { icon: '◐', label: 'Theme: auto (follows system). Click for light.' },
    light: { icon: '☀', label: 'Theme: light. Click for dark.' },
    dark: { icon: '☾', label: 'Theme: dark. Click for auto.' },
  };
  const current = $derived(byMode[theme.mode]);
</script>

<button
  class="theme-toggle icon-only"
  type="button"
  aria-label={current.label}
  title={current.label}
  onclick={() => theme.cycle()}
>
  <span aria-hidden="true">{current.icon}</span>
</button>
