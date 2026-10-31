<script lang="ts">
  import { OPTIONAL_COLUMNS, defaultColumns, type HerdColumnKey } from '../lib/herd';

  // Disclosure with a checkbox per optional column. Endpoint, Status, Resp.
  // rate and Latency are not listed because they are always shown.
  interface Props {
    selected?: HerdColumnKey[];
    onchange?: (keys: HerdColumnKey[]) => void;
  }
  let { selected = [], onchange }: Props = $props();

  let open = $state(false);
  let root: HTMLDivElement | undefined = $state();

  function toggle(key: HerdColumnKey, on: boolean): void {
    const next = OPTIONAL_COLUMNS.map((c) => c.key).filter((k) =>
      k === key ? on : selected.includes(k)
    );
    onchange?.(next);
  }

  function onKey(e: KeyboardEvent): void {
    if (e.key === 'Escape' && open) {
      open = false;
      root?.querySelector<HTMLButtonElement>('.picker-btn')?.focus();
    }
  }

  // Close when a pointer or keyboard focus lands outside, so it behaves like a menu.
  function onWindowPointer(e: Event): void {
    if (open && root && !root.contains(e.target as Node)) open = false;
  }

  function onFocusOut(e: FocusEvent): void {
    const next = e.relatedTarget as Node | null;
    // A null target means focus left the document or went to a non-focusable
    // element; the pointerdown handler covers the click case.
    if (open && next && root && !root.contains(next)) open = false;
  }
</script>

<svelte:window onpointerdown={onWindowPointer} />

<div class="column-picker" bind:this={root} onkeydown={onKey} onfocusout={onFocusOut} role="presentation">
  <button
    type="button"
    class="picker-btn"
    aria-expanded={open}
    aria-controls="herd-column-picker"
    onclick={() => (open = !open)}
  >Columns<span aria-hidden="true"> ▾</span></button>
  {#if open}
    <div class="picker-pop" id="herd-column-picker" role="group" aria-label="Optional columns">
      {#each OPTIONAL_COLUMNS as c (c.key)}
        <label class="picker-row" title={c.hint}>
          <input
            type="checkbox"
            checked={selected.includes(c.key)}
            onchange={(e) => toggle(c.key, e.currentTarget.checked)}
          />
          <span>{c.label}</span>
        </label>
      {/each}
      <p class="picker-note only-narrow">
        On screens under 600px only Endpoint, Status, Resp. rate and Latency show, whatever is ticked here.
      </p>
      <button type="button" class="picker-reset" onclick={() => onchange?.(defaultColumns())}>Reset to default</button>
    </div>
  {/if}
</div>
