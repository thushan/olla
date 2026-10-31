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
    if (!open || !root) return;
    // A press inside the menu can move focus anywhere: nowhere in Safari (it
    // does not focus checkboxes), or to the nearest focusable ancestor such as
    // the tabindex=0 panel in Chromium. Neither is the user leaving the menu.
    if (pressInside) return;
    if (next === null || !root.contains(next)) open = false;
  }
  let pressInside = false;
</script>

<svelte:window onpointerdown={onWindowPointer} onpointerup={() => (pressInside = false)} onpointercancel={() => (pressInside = false)} />

<div class="column-picker" bind:this={root} onkeydown={onKey} onfocusout={onFocusOut} onpointerdown={() => (pressInside = true)} role="presentation">
  <button
    type="button"
    class="picker-btn"
    aria-expanded={open}
    aria-controls="herd-column-picker"
    onclick={() => (open = !open)}
  >Columns<span aria-hidden="true"> ▾</span></button>
  {#if open}
    <!-- Mousedown on label text or padding would otherwise shift focus to the
         focusable panel behind the menu, which reads as focus leaving it. Keep
         focus where it is; the click still toggles the row. -->
    <div
      class="picker-pop"
      id="herd-column-picker"
      role="group"
      aria-label="Optional columns"
      onmousedown={(e) => {
        if (!(e.target instanceof HTMLInputElement)) e.preventDefault();
      }}
    >
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
