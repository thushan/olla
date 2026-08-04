// Shared factory for the three poll stores. Each store wraps a single endpoint
// and exposes the same getter shape (data/status/lastUpdated/error) so the
// panels can render states uniformly. The status state machine lives here:
// loading -> ok on first success; error/stale on failure, with the last
// known-good data preserved and greyed rather than blanked.

import type { PollStatus } from '../types';
import { pollScheduler, STALE_MULTIPLIER } from '../poll-scheduler';

export interface PollStoreOptions<T> {
  /** scheduler job name */
  name: string;
  /** absolute URL to fetch */
  url: string;
  /** foreground poll interval */
  intervalMs: number;
  /** Fires on every completed tick, success or failure. ok=true for 200/304
   *  (data is the parsed body on 200, null on 304); ok=false on failure.
   *  Optional and additive, so stores that don't care (endpoints/models) are
   *  unaffected. The overview store uses this to push per-tick samples into
   *  the history ring buffer, including failed ticks so the chart can render
   *  outage markers. */
  onTick?: (data: T | null, ok: boolean) => void;
}

export interface PollStore<T> {
  readonly name: string;
  readonly intervalMs: number;
  readonly data: T | null;
  readonly status: PollStatus;
  readonly error: Error | null;
  readonly lastUpdated: Date | null;
  readonly hasData: boolean;
  /** Activate this store's scheduler job and fire an immediate tick. Pair
   *  with stop() on panel unmount so inactive panels stop polling. */
  start(): void;
  /** Deactivate this store's job, clear its timer and abort any in-flight tick. */
  stop(): void;
  refresh(): void;
}

export function createPollStore<T>(opts: PollStoreOptions<T>): PollStore<T> {
  const { name, url, intervalMs, onTick } = opts;

  let data: T | null = $state(null);
  let status: PollStatus = $state('loading');
  let error: Error | null = $state(null);
  let lastUpdated: Date | null = $state(null);
  let lastSuccessAt: number = $state(0); // epoch ms, for stale detection
  let etag: string | null = $state(null); // for If-None-Match / 304 handling

  async function tick(signal: AbortSignal): Promise<void> {
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (etag) headers['If-None-Match'] = etag;

    let resp: Response;
    try {
      resp = await fetch(url, { headers, signal, cache: 'no-store' });
    } catch (e) {
      if (e instanceof Error && e.name === 'AbortError') return; // superseded by a newer tick
      onFailure(e instanceof Error ? e : new Error(String(e || 'network error')));
      return;
    }

    if (resp.status === 304) {
      // Body unchanged. The ETag is deliberately stable when the model set
      // hasn't changed, so the panel keeps the last known-good data and the
      // success clock advances.
      const newEtag = resp.headers.get('ETag');
      if (newEtag) etag = newEtag;
      onSuccess(null);
      return;
    }

    if (!resp.ok) {
      onFailure(new Error(`HTTP ${resp.status}`));
      return;
    }

    let body: T;
    try {
      body = (await resp.json()) as T;
    } catch {
      onFailure(new Error('invalid JSON from status API'));
      return;
    }

    const newEtag = resp.headers.get('ETag');
    if (newEtag) etag = newEtag;

    onSuccess(body);
  }

  // Shared by the 200 and 304 branches. A null body (304 path) leaves the
  // cached data untouched but still advances the success clock.
  function onSuccess(body: T | null): void {
    if (body !== null) data = body;
    error = null;
    status = 'ok';
    lastUpdated = new Date();
    lastSuccessAt = Date.now();
    onTick?.(body, true);
  }

  function onFailure(err: Error): void {
    error = err;
    const hasPrior = data !== null;
    const ageMs = Date.now() - lastSuccessAt;
    // Once the last success is older than 3x the interval (several consecutive
    // failures), escalate from error to stale.
    if (hasPrior && ageMs > intervalMs * STALE_MULTIPLIER) {
      status = 'stale';
    } else {
      // First-ever poll failed or still within the stale window: we want the
      // panel to surface the error rather than sit on a blank skeleton.
      status = 'error';
    }
    onTick?.(null, false);
  }

  // Register with the shared scheduler. Caller (App.svelte) starts it.
  pollScheduler.register(name, intervalMs, tick);

  return {
    get name() {
      return name;
    },
    get intervalMs() {
      return intervalMs;
    },
    get data() {
      return data;
    },
    get status() {
      return status;
    },
    get error() {
      return error;
    },
    get lastUpdated() {
      return lastUpdated;
    },
    /** True if the store has at least one successful poll under its belt. */
    get hasData() {
      return data !== null;
    },
    /** Activate the scheduler job and fire an immediate tick. Called by the
     *  owning panel on mount so a tab switch doesn't sit on stale data. */
    start() {
      pollScheduler.start(name);
    },
    /** Deactivate the scheduler job; in-flight ticks are aborted by the scheduler. */
    stop() {
      pollScheduler.stop(name);
    },
    /** Force an immediate refresh (used by "retry now" buttons). */
    refresh() {
      pollScheduler.refresh(name);
    },
  };
}
