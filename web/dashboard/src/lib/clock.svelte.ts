// Single ticking instant for any component that needs to re-render
// time-derived values (relative "Xs ago", uptime, "in 30s"). Status
// surfaces render absolute instants from the API as relative strings, and
// those go stale every second; rather than give each panel its own
// setInterval (spec §7.3 forbids that), this module is the one shared
// clock. Reads via now() are reactive when consumed inside $derived.

import { pollScheduler } from './poll-scheduler';

const CLOCK_INTERVAL_MS = 1000;
const CLOCK_JOB_NAME = 'clock';

let now = $state(Date.now());

function tick(): void {
  now = Date.now();
}

let registered = false;
export function startClock(): void {
  if (registered) return;
  registered = true;
  pollScheduler.register(CLOCK_JOB_NAME, CLOCK_INTERVAL_MS, tick);
  // Kick once immediately so the first paint isn't up to 1s stale.
  tick();
}

/** Live "now" in epoch ms; read inside $derived so panels re-render per tick. */
export function getNow(): number {
  return now;
}
