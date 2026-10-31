<script lang="ts">
  // Single component enforcing "glyph + colour + text" for every status
  // indicator. Never colour alone (colour-blind / greyscale).
  //
  // When `issues` is non-empty the pill becomes a focusable button that shows
  // the reasons in a tooltip on hover and on keyboard/touch focus. The tooltip
  // is position:fixed because the herd table lives in an overflow scroller
  // that would clip an absolutely positioned child.
  interface Props {
    status?: string;
    issues?: string[];
    // Extra "label: value" lines under the issues (probe latency, next check).
    lines?: string[];
  }
  let { status = 'healthy', issues = [], lines = [] }: Props = $props();

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

  const tipId = $props.id();
  let open = $state(false);
  let pos = $state({ top: 0, left: 0 });
  let btn: HTMLButtonElement | undefined = $state();

  const TIP_W = 240;
  function show(): void {
    if (btn) {
      const r = btn.getBoundingClientRect();
      // Clamp so the tip never runs off the right edge on a 390px screen.
      const left = Math.max(8, Math.min(r.left, window.innerWidth - TIP_W - 8));
      pos = { top: r.bottom + 4, left };
    }
    open = true;
  }
  function hide(): void {
    open = false;
  }
  function onKey(e: KeyboardEvent): void {
    if (e.key === 'Escape' && open) {
      open = false;
      e.stopPropagation();
    }
  }
</script>

{#if issues.length > 0}
  <button
    bind:this={btn}
    type="button"
    class="status-tag st-{meta.cls} has-issues"
    aria-describedby={tipId}
    onmouseenter={show}
    onmouseleave={hide}
    onfocus={show}
    onblur={hide}
    onkeydown={onKey}
  >
    <span class="glyph g-{meta.cls}" aria-hidden="true">{meta.glyph}</span>{meta.label}
  </button>
  <!-- Always in the DOM so aria-describedby resolves for screen readers;
       visibility is toggled rather than the node. -->
  <span
    id={tipId}
    role="tooltip"
    class="status-tip"
    class:open
    style="top:{pos.top}px;left:{pos.left}px;width:{TIP_W}px"
  >
    <strong>Issues</strong>
    <ul>
      {#each issues as i}<li>{i}</li>{/each}
    </ul>
    {#each lines as l}<span class="tip-line">{l}</span>{/each}
  </span>
{:else}
  <span class="status-tag st-{meta.cls}">
    <span class="glyph g-{meta.cls}" aria-hidden="true">{meta.glyph}</span>{meta.label}
  </span>
{/if}
