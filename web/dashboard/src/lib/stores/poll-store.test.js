import { describe, it, expect, beforeEach, vi } from 'vitest';

// Regression coverage for finding 3: the companion fetch (e.g.
// /internal/stats/models) was only ever attempted after a 200 from the
// primary URL. In steady state the primary's ETag is deliberately stable
// once the model set stops changing, so it 304s continuously and the
// companion - which carries the numbers that move every tick (requests,
// success rate, latency) - never runs again. The panel looks alive
// (lastUpdated keeps advancing) while the figures freeze.

beforeEach(() => {
  vi.resetModules();
  vi.restoreAllMocks();
});

function jsonResponse(body, { status = 200, etag } = {}) {
  const headers = new Map();
  if (etag) headers.set('ETag', etag);
  return {
    status,
    ok: status >= 200 && status < 300,
    headers: { get: (k) => headers.get(k) ?? null },
    json: async () => body,
  };
}

/** Builds a fetch mock that 304s the primary from the second call onward. */
function makeFetch({ primaryBody, companionBodyFor }) {
  let primaryCalls = 0;
  let companionCalls = 0;
  return vi.fn(async (url) => {
    if (String(url).startsWith('/primary')) {
      primaryCalls++;
      if (primaryCalls === 1) return jsonResponse(primaryBody, { etag: '"v1"' });
      return jsonResponse(null, { status: 304, etag: '"v1"' });
    }
    companionCalls++;
    return jsonResponse(companionBodyFor(companionCalls));
  });
}

describe('createPollStore: companion refresh across a 304', () => {
  it('still fetches and merges the companion after the primary 304s', async () => {
    const { createPollStore } = await import('./poll-store.svelte.js');

    global.fetch = makeFetch({
      primaryBody: { models: ['a'] },
      companionBodyFor: (n) => ({ requests: n * 10 }),
    });

    const merge = (payload, companion) => ({ ...payload, requests: companion?.requests ?? null });

    const store = createPollStore({
      name: 'test-304-companion',
      url: '/primary',
      intervalMs: 1000,
      companionUrl: '/companion',
      companionMerge: merge,
    });

    store.refresh();
    await vi.waitFor(() => expect(store.data?.requests).toBe(10));
    expect(store.data.models).toEqual(['a']);

    // Second poll: primary 304s. The companion has no ETag and its own
    // number keeps moving - this is the freeze this test guards against.
    store.refresh();
    await vi.waitFor(() => expect(store.data?.requests).toBe(20));
    // Cached primary payload must survive the re-merge untouched.
    expect(store.data.models).toEqual(['a']);
    expect(store.status).toBe('ok');
  });

  it('keeps the last good merged data if the companion fetch fails on a 304', async () => {
    const { createPollStore } = await import('./poll-store.svelte.js');

    let primaryCalls = 0;
    global.fetch = vi.fn(async (url) => {
      if (String(url).startsWith('/primary')) {
        primaryCalls++;
        if (primaryCalls === 1) return jsonResponse({ models: ['a'] }, { etag: '"v1"' });
        return jsonResponse(null, { status: 304, etag: '"v1"' });
      }
      // Companion fails every time it's called.
      throw new Error('network error');
    });

    const merge = (payload, companion) => ({ ...payload, requests: companion?.requests ?? 'MISSING' });

    const store = createPollStore({
      name: 'test-304-companion-fail',
      url: '/primary',
      intervalMs: 1000,
      companionUrl: '/companion',
      companionMerge: merge,
    });

    store.refresh();
    await vi.waitFor(() => expect(store.data).not.toBeNull());
    // First poll's companion also failed (fetch always throws), so the merge
    // ran once with a null companion.
    expect(store.data.requests).toBe('MISSING');

    store.refresh();
    // Companion has only just started failing (one streak tick in), so it
    // hasn't crossed the escalation window yet - primary is genuinely fine.
    await vi.waitFor(() => expect(store.status).toBe('ok'));
    // Data object reference is unchanged: onSuccess(null, ...) must not have
    // replaced it with a freshly-merged-against-null payload.
    expect(store.data.requests).toBe('MISSING');
    expect(store.data.models).toEqual(['a']);
  });

  // Regression coverage for finding 6: onSuccess(null, true) on a 304-with-
  // failed-companion used to clear `error`, set status='ok' and reset
  // `lastSuccessAt` unconditionally - the very clock onFailure consults to
  // escalate to 'stale'. A companion that never recovers therefore reported
  // 'ok' forever, with frozen request/latency figures and no banner: there
  // was no exit. This asserts the exit exists.
  it('escalates to stale once the companion has been failing longer than the stale window, even though the primary keeps 304-ing', async () => {
    vi.useFakeTimers();
    try {
      const { createPollStore } = await import('./poll-store.svelte.js');

      let primaryCalls = 0;
      global.fetch = vi.fn(async (url) => {
        if (String(url).startsWith('/primary')) {
          primaryCalls++;
          if (primaryCalls === 1) return jsonResponse({ models: ['a'] }, { etag: '"v1"' });
          return jsonResponse(null, { status: 304, etag: '"v1"' });
        }
        // Companion fails on every call, forever.
        throw new Error('network error');
      });

      const merge = (payload, companion) => ({ ...payload, requests: companion?.requests ?? 'MISSING' });

      const store = createPollStore({
        name: 'test-304-companion-persistent-fail',
        url: '/primary',
        intervalMs: 1000,
        companionUrl: '/companion',
        companionMerge: merge,
      });

      store.refresh();
      await vi.waitFor(() => expect(store.data).not.toBeNull());
      expect(store.status).toBe('ok');

      // Still within the stale window (3x interval = 3000ms): a couple of
      // quick companion failures must not yet escalate.
      await vi.advanceTimersByTimeAsync(500);
      store.refresh();
      await vi.waitFor(() => expect(store.data.requests).toBe('MISSING'));
      expect(store.status).toBe('ok');

      // Push the companion failure streak well past the stale window. The
      // primary is STILL 304-ing happily throughout - this must not matter.
      await vi.advanceTimersByTimeAsync(4000);
      store.refresh();
      await vi.waitFor(() => expect(store.status).toBe('stale'));
      // Last known-good data is preserved, not blanked.
      expect(store.data.models).toEqual(['a']);
    } finally {
      vi.useRealTimers();
    }
  });
});
