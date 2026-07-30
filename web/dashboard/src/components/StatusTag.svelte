<script>
  // Single component enforcing "glyph + colour + text" for every status
  // indicator. Never colour alone (colour-blind / greyscale).
  let { status = 'healthy' } = $props();

  const STATUS = {
    healthy: { glyph: '●', cls: 'green', label: 'healthy' },
    degraded: { glyph: '◐', cls: 'amber', label: 'degraded' },
    unhealthy: { glyph: '◐', cls: 'amber', label: 'unhealthy' },
    offline: { glyph: '○', cls: 'red', label: 'offline' },
    critical: { glyph: '○', cls: 'red', label: 'critical' },
    unknown: { glyph: '○', cls: 'neutral', label: 'unknown' },
  };

  const meta = $derived(STATUS[status] ?? { glyph: '○', cls: 'neutral', label: status || 'unknown' });
</script>

<span class="status-tag st-{meta.cls}">
  <span class="glyph g-{meta.cls}" aria-hidden="true">{meta.glyph}</span>{meta.label}
</span>
