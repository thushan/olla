<script lang="ts">
  // Single component enforcing "glyph + colour + text" for every status
  // indicator. Never colour alone (colour-blind / greyscale).
  interface Props {
    status?: string;
  }
  let { status = 'healthy' }: Props = $props();

  type Meta = { glyph: string; cls: string; label: string };

  const STATUS: Record<string, Meta> = {
    healthy: { glyph: '●', cls: 'green', label: 'healthy' },
    degraded: { glyph: '◐', cls: 'amber', label: 'degraded' },
    unhealthy: { glyph: '◐', cls: 'amber', label: 'unhealthy' },
    offline: { glyph: '○', cls: 'red', label: 'offline' },
    critical: { glyph: '○', cls: 'red', label: 'critical' },
    unknown: { glyph: '○', cls: 'neutral', label: 'unknown' },
  };

  const meta: Meta = $derived(STATUS[status] ?? { glyph: '○', cls: 'neutral', label: status || 'unknown' });
</script>

<span class="status-tag st-{meta.cls}">
  <span class="glyph g-{meta.cls}" aria-hidden="true">{meta.glyph}</span>{meta.label}
</span>
