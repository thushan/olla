// Shared "jump to endpoint" helper used by Overview's glance table, Models'
// endpoint pills, and the URL-restore path. Swaps to the Endpoints panel,
// waits for the target row to render (covering the first-navigation fetch
// race), then scrolls it into view, focuses it, and applies a brief
// accent-tint flash so the operator can spot which row was the target amid a
// long fleet.
//
// Both call sites used to carry their own copy of this logic; the duplicate
// drifted (Overview omitted the retry loop, so its first jump after a cold
// load silently no-op'd). One helper closes that gap.

import { tick } from 'svelte';
import { stableId } from './dom-id';
import { navigation } from './stores/navigation.svelte';
import { pushRoute } from './router';

const EP_PREFIX = 'ep-';
const FLASH_CLASS = 'ep-flash';
// Hold the tint for ~1s then fade over the rest; the matching CSS animation
// in components.css uses the same 1.8s envelope. Couple them via a single
// constant so the class is removed exactly when the animation lands on its
// transparent end-state, never while the tint is still visible.
const FLASH_HOLD_MS = 1800;
// First-navigation race: EndpointsPanel starts its store on mount and the
// fetch is async, so the target row may not exist immediately after the panel
// swap. ~8 tries x ~50ms caps the wait near 400ms and returns the instant
// the row appears, without a busy windmill.
const RETRY_ATTEMPTS = 8;
const RETRY_DELAY_MS = 50;

// Jump from a click handler, where we hold the row's raw id (the server's
// opaque endpoint token, or a url/name fallback). Hashes the id for the DOM
// lookup, pushes a history entry so back/forward returns here, then reveals
// the row.
export async function jumpToEndpoint(rawId: string): Promise<void> {
  const key = stableId(rawId);
  navigation.set('endpoints');
  pushRoute({ panel: 'endpoints', endpointKey: key });
  await revealRow(`${EP_PREFIX}${key}`);
}

// Restore variant for URL-driven navigation (initial load, back/forward).
// The route already carries the stableId and is already the current history
// entry, so we skip the hash push and go straight to revealing the row.
export async function jumpToEndpointKey(endpointKey: string): Promise<void> {
  navigation.set('endpoints');
  await revealRow(`${EP_PREFIX}${endpointKey}`);
}

// tick() flushes the panel swap (App.svelte's {#if} mounts EndpointsPanel);
// the bounded retry then covers the window between mount and the row
// appearing in the DOM once the fetch resolves.
async function revealRow(domId: string): Promise<void> {
  await tick();
  let el = document.getElementById(domId);
  for (let attempt = 0; !el && attempt < RETRY_ATTEMPTS; attempt++) {
    await new Promise((r) => setTimeout(r, RETRY_DELAY_MS));
    el = document.getElementById(domId);
  }
  if (!el) return;
  el.scrollIntoView({ block: 'center' });
  el.focus();
  flashRow(el);
}

// Only one flash timer is live at a time. Tracking it here means a re-flash
// (same row clicked twice, or back/forward landing on the row that's still
// fading) cancels the pending removal first, so the old timer can't fire
// partway through the new hold and strip the class early.
let flashTimer: ReturnType<typeof setTimeout> | null = null;

export function flashRow(el: HTMLElement): void {
  // Only one row is ever lit at a time: clear any in-flight flash on other
  // rows so a second jump moves the highlight rather than stacking it.
  document.querySelectorAll(`.${FLASH_CLASS}`).forEach((node) => {
    node.classList.remove(FLASH_CLASS);
  });
  if (flashTimer !== null) {
    clearTimeout(flashTimer);
    flashTimer = null;
  }
  // Restart the animation if the SAME row is re-targeted mid-flash. Remove,
  // force a synchronous reflow so the browser can't coalesce the class
  // toggle, then re-add.
  el.classList.remove(FLASH_CLASS);
  void el.offsetWidth;
  el.classList.add(FLASH_CLASS);
  flashTimer = setTimeout(() => {
    el.classList.remove(FLASH_CLASS);
    flashTimer = null;
  }, FLASH_HOLD_MS);
}
