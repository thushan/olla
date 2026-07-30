<script>
  // Single component enforcing "glyph + colour + text" for every status /
  // circuit-breaker indicator. Never colour alone (colour-blind / greyscale).
  let { status = 'healthy', kind = 'status' } = $props();

  // Two maps with identical shape so this component serves both signal types
  // without callers reaching into colour classes directly.
  const STATUS = {
    healthy: { glyph: '●', cls: 'green', label: 'healthy' },
    degraded: { glyph: '◐', cls: 'amber', label: 'degraded' },
    unhealthy: { glyph: '◐', cls: 'amber', label: 'unhealthy' },
    offline: { glyph: '○', cls: 'red', label: 'offline' },
    critical: { glyph: '○', cls: 'red', label: 'critical' },
    unknown: { glyph: '○', cls: 'neutral', label: 'unknown' },
  };
  const CB = {
    closed: { glyph: '●', cls: 'green', label: 'closed' },
    'half-open': { glyph: '◐', cls: 'amber', label: 'half-open' },
    open: { glyph: '○', cls: 'red', label: 'open' },
  };

  const map = $derived(kind === 'breaker' ? CB : STATUS);
  const meta = $derived(map[status] ?? { glyph: '○', cls: 'neutral', label: status || 'unknown' });
</script>

<span class="status-tag st-{meta.cls}">
  <span class="glyph g-{meta.cls}" aria-hidden="true">{meta.glyph}</span>{meta.label}
</span>
