// Pollers for the Overview "Runtime" cards: sticky sessions, translators and
// the Go process. Slow-moving counters, so 15s like the models store. The
// shared scheduler already backs off while the tab is hidden and refreshes on
// return, so nothing extra is needed here. They are started and stopped by
// the cards that read them, not by App.
import type { ProcessStatsResponse, StickyStatsResponse, TranslatorStatsResponse } from '../types';
import { createPollStore } from './poll-store.svelte';

const RUNTIME_INTERVAL_MS = 15000;

export const sticky = createPollStore<StickyStatsResponse>({
  name: 'sticky',
  url: '/internal/stats/sticky',
  intervalMs: RUNTIME_INTERVAL_MS,
});

export const translators = createPollStore<TranslatorStatsResponse>({
  name: 'translators',
  url: '/internal/stats/translators',
  intervalMs: RUNTIME_INTERVAL_MS,
});

export const processStats = createPollStore<ProcessStatsResponse>({
  name: 'process',
  url: '/internal/process',
  intervalMs: RUNTIME_INTERVAL_MS,
});
