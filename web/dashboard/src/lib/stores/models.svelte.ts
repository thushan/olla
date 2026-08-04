// /internal/status/models poller. Drives ModelsPanel. Detailed/grouped form
// is requested so the panel can render family groups without client re-group.
//
// Per-model request/latency stats (previously sourced from a secondary fetch
// to /internal/stats/models) are intentionally NOT fetched here: nothing on
// the proxy path calls RecordModelRequest, so those figures are always zero
// and presenting them would read as "never used" when the truth is "never
// counted". Wiring per-model request counting on the proxy path is deferred
// to a later change.
import type { ModelStatusResponse } from '../types';
import { createPollStore } from './poll-store.svelte';

const MODELS_INTERVAL_MS = 15000;
// detailed=true (not =1): the handler compares against the literal "true".
// group=family populates model_groups so the panel can render families.
const STATUS_URL = '/internal/status/models?detailed=true&group=family';

export const models = createPollStore<ModelStatusResponse>({
  name: 'models',
  url: STATUS_URL,
  intervalMs: MODELS_INTERVAL_MS,
});
