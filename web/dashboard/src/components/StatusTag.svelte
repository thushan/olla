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

  // A stale-data wrapper applies CSS `filter` to an ancestor, which makes it
  // the containing block for position:fixed and throws viewport-coordinate
  // placement off. Parenting the tip on <body> sidesteps every such ancestor;
  // the theme tokens live on :root so they still apply there.
  function portal(node: HTMLElement): { destroy(): void } {
    document.body.appendChild(node);
    return { destroy: () => node.remove() };
  }

  const tipId = $props.id();
  let open = $state(false);
  let pos = $state({ top: 0, left: 0, width: 240 });
  let btn: HTMLButtonElement | undefined = $state();
  let tip: HTMLSpanElement | undefined = $state();

  const TIP_W = 240;
  const MARGIN = 8;
  // Long enough to cross the gap between pill and tip without it flickering
  // shut, short enough not to feel sticky.
  const CLOSE_DELAY_MS = 120;

  // Measured at open time (the tip is laid out even while visibility:hidden):
  // below the pill when it fits, above when it doesn't, and clamped inside
  // the viewport horizontally.
  function place(): void {
    if (!btn) return;
    const r = btn.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const width = Math.min(TIP_W, vw - 2 * MARGIN);
    const h = tip?.offsetHeight ?? 0;
    const left = Math.max(MARGIN, Math.min(r.left, vw - width - MARGIN));
    let top = r.bottom + 4;
    if (top + h > vh - MARGIN) {
      top = r.top - 4 - h >= MARGIN ? r.top - 4 - h : Math.max(MARGIN, vh - h - MARGIN);
    }
    pos = { top, left, width };
  }

  // Open while the pill or the tip is hovered or focused; closing waits a
  // beat so moving the pointer from one to the other keeps it open.
  let hovered = false;
  let focused = false;
  let timer: ReturnType<typeof setTimeout> | null = null;
  function cancelTimer(): void {
    if (timer !== null) clearTimeout(timer);
    timer = null;
  }
  function sync(): void {
    cancelTimer();
    if (hovered || focused) {
      if (!open) {
        place();
        open = true;
      }
    } else if (open) {
      timer = setTimeout(close, CLOSE_DELAY_MS);
    }
  }
  function close(): void {
    cancelTimer();
    open = false;
  }
  function setHover(v: boolean): void {
    hovered = v;
    sync();
  }
  function setFocus(v: boolean): void {
    focused = v;
    sync();
  }

  // Pointer state before focus fires, so a click on a hover-opened tip closes
  // it and a first tap on touch (no hover) opens it.
  let wasOpen = false;
  function onClick(): void {
    if (wasOpen) close();
    else if (!open) {
      place();
      open = true;
    }
    wasOpen = false;
  }

  // The tip is position:fixed where it was placed; close it if the page moves
  // under it instead of leaving it adrift.
  $effect(() => {
    if (!open) return;
    window.addEventListener('scroll', close, true);
    window.addEventListener('resize', close);
    return () => {
      window.removeEventListener('scroll', close, true);
      window.removeEventListener('resize', close);
    };
  });

  $effect(() => () => cancelTimer());

  // The pill and tip are destroyed when issues clear, without a mouseleave or
  // blur, so reset everything or the tip would reopen the moment issues return.
  $effect(() => {
    if (issues.length === 0) {
      cancelTimer();
      hovered = false;
      focused = false;
      wasOpen = false;
      open = false;
    }
  });

  function onKey(e: KeyboardEvent): void {
    if (e.key === 'Escape' && open) {
      close();
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
    onmouseenter={() => setHover(true)}
    onmouseleave={() => setHover(false)}
    onpointerdown={() => (wasOpen = open)}
    onclick={onClick}
    onfocus={() => setFocus(true)}
    onblur={() => setFocus(false)}
    onkeydown={onKey}
  >
    <span class="glyph g-{meta.cls}" aria-hidden="true">{meta.glyph}</span>{meta.label}
  </button>
  <!-- Always in the DOM so aria-describedby resolves for screen readers;
       visibility is toggled rather than the node. -->
  <span
    bind:this={tip}
    use:portal
    id={tipId}
    role="tooltip"
    class="status-tip"
    class:open
    style="top:{pos.top}px;left:{pos.left}px;width:{pos.width}px"
    onmouseenter={() => setHover(true)}
    onmouseleave={() => setHover(false)}
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
