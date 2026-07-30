// Shared factory for the three poll stores. Each store wraps a single endpoint
// and exposes the same getter shape (data/status/lastUpdated/error) so the
// panels can render states uniformly. The status state machine (§7.3) lives
// here: loading -> ok on first success; error/stale on failure, with the last
// known-good data preserved and greyed rather than blanked.

import { pollScheduler, STALE_MULTIPLIER } from '../poll-scheduler.js';

/**
 * @param {object} opts
 * @param {string} opts.name            scheduler job name
 * @param {string} opts.url             absolute URL to fetch
 * @param {number} opts.intervalMs      foreground poll interval
 * @param {string=} opts.companionUrl   secondary URL fetched after each primary success
 * @param {(payload: object, companion: object|null) => object} opts.companionMerge
 *   merges the companion response into the primary payload; companion
 *   failure is non-fatal (called with null) so the primary surface still
 *   updates
 */
export function createPollStore({ name, url, intervalMs, companionUrl, companionMerge }) {
  let data = $state(null);
  let status = $state('loading');
  let error = $state(null);
  let lastUpdated = $state(null); // Date of last successful poll
  let lastSuccessAt = $state(0); // epoch ms, for stale detection
  let etag = $state(null); // for If-None-Match / 304 handling

  // Companion freshness is tracked on its own clock. The primary can 304
  // forever (its ETag is stable once the model set stops changing) while the
  // companion - which carries the figures that actually move every tick -
  // fails outright. That must not hide behind a healthy primary: a companion
  // failure must never reset the PRIMARY's success clock (lastSuccessAt),
  // and a persistently failing companion must eventually surface through
  // `status` on its own, rather than reporting 'ok' forever with frozen
  // numbers and no banner.
  let companionFailingSince = $state(0); // epoch ms the current failure streak began, 0 = not failing

  function onCompanionSuccess() {
    companionFailingSince = 0;
  }

  function onCompanionFailure() {
    if (companionFailingSince === 0) companionFailingSince = Date.now();
  }

  // A companion stuck failing for longer than the same escalation window
  // used for primary failures reads as stale even though the primary itself
  // is answering fine - the merged data it would have supplied is frozen.
  function companionIsStale() {
    return companionFailingSince !== 0 && Date.now() - companionFailingSince > intervalMs * STALE_MULTIPLIER;
  }

  async function tick(signal) {
    const headers = { Accept: 'application/json' };
    if (etag) headers['If-None-Match'] = etag;

    let resp;
    try {
      resp = await fetch(url, { headers, signal, cache: 'no-store' });
    } catch (e) {
      if (e && e.name === 'AbortError') return; // superseded by a newer tick
      onFailure(e instanceof Error ? e : new Error(String(e || 'network error')));
      return;
    }

    if (resp.status === 304) {
      // Body unchanged (the ETag is deliberately stable when the model set
      // hasn't changed). The companion endpoint has no ETag of its own and
      // carries the figures that DO move every tick (request counts, success
      // rate, latency), so it must still be re-fetched here - otherwise those
      // numbers freeze at whatever they were the last time the primary body
      // changed, even though `lastUpdated` keeps advancing and the panel
      // looks live.
      const newEtag = resp.headers.get('ETag');
      if (newEtag) etag = newEtag;

      if (companionUrl && companionMerge && data !== null) {
        let companion;
        try {
          companion = await fetchCompanion(signal);
        } catch (e) {
          return; // aborted, superseded by a newer tick
        }
        if (companion !== null) {
          // Re-merge the fresh companion onto the CACHED primary payload -
          // the primary genuinely hasn't changed, only the companion stats
          // need refreshing.
          onCompanionSuccess();
          onSuccess(companionMerge(data, companion), true);
        } else {
          // Companion fetch failed: keep the last good merged data rather
          // than clobbering it via companionMerge(data, null). The PRIMARY
          // did genuinely 304 (it is healthy), so its own success clock still
          // advances via onSuccess - only the companion's own failure clock
          // tracks this, so a persistent companion outage can still escalate
          // even while the primary keeps answering.
          onCompanionFailure();
          onSuccess(null, true);
        }
      } else {
        onSuccess(null, true);
      }
      return;
    }

    if (!resp.ok) {
      onFailure(new Error(`HTTP ${resp.status}`));
      return;
    }

    let body;
    try {
      body = await resp.json();
    } catch (e) {
      onFailure(new Error('invalid JSON from status API'));
      return;
    }

    const newEtag = resp.headers.get('ETag');
    if (newEtag) etag = newEtag;

    // Companion fetch (e.g. /internal/stats/models alongside status/models).
    // Failures are non-fatal: primary payload still updates; the merge gets
    // null so callers can fall back to "no stats yet" rather than blanking.
    if (companionUrl) {
      let companion;
      try {
        companion = await fetchCompanion(signal);
      } catch (e) {
        return; // aborted, superseded by a newer tick
      }
      if (companion !== null) onCompanionSuccess();
      else onCompanionFailure();
      if (companionMerge) body = companionMerge(body, companion);
    }

    onSuccess(body, false);
  }

  // Shared by the 200 and 304 branches. Resolves to the parsed companion body,
  // or null on a soft failure (non-OK response, network error, bad JSON) so
  // callers can fall back to "no stats" rather than blanking the primary.
  // Rethrows AbortError so callers can bail out exactly like the primary
  // fetch does when superseded by a newer tick.
  async function fetchCompanion(signal) {
    try {
      const cresp = await fetch(companionUrl, { headers: { Accept: 'application/json' }, signal, cache: 'no-store' });
      return cresp.ok ? await cresp.json() : null;
    } catch (e) {
      if (e && e.name === 'AbortError') throw e;
      return null;
    }
  }

  function onSuccess(body, is304) {
    if (body !== null) data = body;
    error = null;
    // A companion stuck failing long enough reads as stale even though the
    // primary just answered fine - otherwise a dead companion hides forever
    // behind a healthy primary 304 loop, reporting 'ok' with frozen figures
    // and no banner (finding 6).
    status = companionIsStale() ? 'stale' : 'ok';
    lastUpdated = new Date();
    lastSuccessAt = Date.now();
  }

  function onFailure(err) {
    error = err;
    const hasPrior = data !== null;
    const ageMs = Date.now() - lastSuccessAt;
    // Once the last success is older than 3x the interval (several consecutive
    // failures), escalate from error to stale.
    if (hasPrior && ageMs > intervalMs * STALE_MULTIPLIER) {
      status = 'stale';
    } else if (hasPrior) {
      status = 'error';
    } else {
      // First-ever poll failed: we have nothing to show, still "error" so the
      // panel doesn't sit on a blank loading skeleton forever.
      status = 'error';
    }
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
    /** Force an immediate refresh (used by "retry now" buttons). */
    refresh() {
      pollScheduler.refresh(name);
    },
  };
}
