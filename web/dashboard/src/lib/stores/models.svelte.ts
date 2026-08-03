// /internal/status/models poller. Drives ModelsPanel. Detailed/grouped form
// is requested so the panel can render family groups without client re-group.
//
// Per-model request/latency stats (previously sourced from a secondary fetch
// to /internal/stats/models) are intentionally NOT fetched this PR: nothing
// on the proxy path calls RecordModelRequest on main, so those figures are
// always zero and presenting them would read as "never used" when the truth
// is "never counted" (spec §4.3). Wiring that is PR 2 proxy-engine scope.
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
