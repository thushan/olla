<script>
  import ThemeToggle from '../components/ThemeToggle.svelte';
  import ollaThumb from '../assets/logo/olla-thumb.png';
  import { getNow } from '../lib/clock.svelte.js';

  // Live clock ticking once per second, grounding the operator that "this
  // view is live, the data is from N seconds ago". Sourced from the shared
  // clock (spec §7.3: the scheduler owns every timer in the SPA) rather than
  // a component-local setInterval, so it gets the visibility backoff for
  // free like every other live-updating value.
  const clockText = $derived(
    new Date(getNow()).toLocaleTimeString('en-AU', { hour12: false })
  );
</script>

<div class="brand-row">
  <div class="brand">
    <img
      class="brand-logo"
      src={ollaThumb}
      width="19"
      height="28"
      alt="Olla llama head logo"
    />
    <h1>OLLA</h1>
    <span class="tag">herd status</span>
  </div>
  <span class="clock" aria-live="off">{clockText}</span>
  <ThemeToggle />
</div>
