// /internal/status poller. Drives the StatusStrip and OverviewPanel.
import { createPollStore } from './poll-store.svelte.js';

const OVERVIEW_INTERVAL_MS = 5000;

// Data endpoint (not a SPA asset), so it lives at /internal/status regardless
// of the SPA base. Olla mounts the proxy listener at /, so this is correct in
// both dev and the embedded /internal/ui/ deployment.
const URL = '/internal/status';

export const overview = createPollStore({
  name: 'overview',
  url: URL,
  intervalMs: OVERVIEW_INTERVAL_MS,
});
