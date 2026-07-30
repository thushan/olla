// /internal/status/endpoints poller. Drives EndpointsPanel + Overview's herd glance.
import { createPollStore } from './poll-store.svelte.js';

const ENDPOINTS_INTERVAL_MS = 5000;
const URL = '/internal/status/endpoints';

export const endpoints = createPollStore({
  name: 'endpoints',
  url: URL,
  intervalMs: ENDPOINTS_INTERVAL_MS,
});
