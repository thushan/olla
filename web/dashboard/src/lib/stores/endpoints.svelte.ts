// /internal/status/endpoints poller. Drives EndpointsPanel + Overview's herd glance.
import type { EndpointStatusResponse } from '../types';
import { createPollStore } from './poll-store.svelte';

const ENDPOINTS_INTERVAL_MS = 5000;
const URL = '/internal/status/endpoints';

export const endpoints = createPollStore<EndpointStatusResponse>({
  name: 'endpoints',
  url: URL,
  intervalMs: ENDPOINTS_INTERVAL_MS,
});
