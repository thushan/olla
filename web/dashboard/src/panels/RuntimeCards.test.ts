import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';

// The stores are module singletons, so each test loads a fresh copy rather than
// inheriting the previous test's data.
let sticky: typeof import('../lib/stores/runtime.svelte').sticky;
let translators: typeof import('../lib/stores/runtime.svelte').translators;
let processStats: typeof import('../lib/stores/runtime.svelte').processStats;
// svelte is reloaded with the modules so mount and the component share one runtime.
let mount: typeof import('svelte').mount;
let unmount: typeof import('svelte').unmount;
let flushSync: typeof import('svelte').flushSync;
let RuntimeCards: typeof import('./RuntimeCards.svelte').default;
beforeEach(async () => {
  vi.resetModules();
  ({ mount, unmount, flushSync } = await import('svelte'));
  ({ sticky, translators, processStats } = await import('../lib/stores/runtime.svelte'));
  RuntimeCards = (await import('./RuntimeCards.svelte')).default;
});

let component: ReturnType<typeof import('svelte').mount> | undefined;
afterEach(() => {
  if (component) unmount(component);
  component = undefined;
  document.body.innerHTML = '';
});

const ok = (body: unknown) => ({ status: 200, ok: true, headers: { get: () => null }, json: async () => body });
const fail = (status: number) => ({ status, ok: false, headers: { get: () => null }, json: async () => ({}) });

const procBody = {
  timestamp: 't',
  memory: { heap_alloc: '4.67 MB', heap_sys: '14.78 MB', heap_inuse: '6.38 MB', memory_pressure: 'MEDIUM' },
  garbage_collection: { avg_gc_pause: '901µs', num_gc_cycles: 9 },
  goroutines: { health_status: 'HEALTHY', count: 31 },
  runtime: { uptime: '22s', go_version: 'go1.24', num_cpu: 8 },
};
const translatorBody = (rows: unknown[]) => ({ timestamp: 't', translators: rows, summary: {} });
const anth = {
  translator_name: 'anthropic', success_rate: '100%', passthrough_rate: '50%', average_latency: '12ms',
  total_requests: 4, successful_requests: 4, failed_requests: 0, passthrough_requests: 2, translation_requests: 2,
  streaming_requests: 3, non_streaming_requests: 1, fallback_no_compatible_endpoints: 1,
  fallback_translator_does_not_support_passthrough: 0, fallback_cannot_passthrough: 0,
};

let routes: Record<string, () => unknown>;
function install() {
  global.fetch = vi.fn(async (url: RequestInfo | URL) => {
    const u = String(url);
    const key = Object.keys(routes).find((k) => u.includes(k))!;
    return routes[key]();
  }) as never;
}
async function settle() {
  sticky.refresh();
  translators.refresh();
  processStats.refresh();
  await vi.waitFor(() => {
    expect(sticky.status).not.toBe('loading');
    expect(translators.status).not.toBe('loading');
    expect(processStats.status).not.toBe('loading');
  });
  flushSync();
}
async function settle2() {
  sticky.refresh();
  translators.refresh();
  processStats.refresh();
  await new Promise((r) => setTimeout(r, 50));
  await vi.waitFor(() => {
    flushSync();
    expect(document.body.textContent).toBeTruthy();
  });
  await new Promise((r) => setTimeout(r, 50));
  flushSync();
}
const txt = () => document.body.textContent ?? '';

describe('RuntimeCards', () => {
  it('renders all three cards from live payloads', async () => {
    routes = {
      '/internal/stats/sticky': () => ok({ enabled: true, active_sessions: 2, insertions: 3, hits: 39, misses: 1, evictions: 0, max_sessions: 1000, idle_ttl_seconds: 600 }),
      '/internal/stats/translators': () => ok(translatorBody([anth])),
      '/internal/process': () => ok(procBody),
    };
    install();
    component = mount(RuntimeCards, { target: document.body });
    await settle();
    expect(document.querySelector('[data-testid="card-sticky"]')).not.toBeNull();
    expect(txt()).toContain('97.5%');
    expect(document.querySelector('[data-testid="card-translator"]')?.textContent).toContain('anthropic');
    expect(document.querySelector('[data-testid="card-process"]')?.textContent).toContain('4.67 MB');
  });

  it('hides sticky and translators when off or empty, and when sticky switches off after being on', async () => {
    let stickyBody: unknown = { enabled: true, active_sessions: 1, hits: 1, misses: 0, max_sessions: 10 };
    routes = {
      '/internal/stats/sticky': () => ok(stickyBody),
      '/internal/stats/translators': () => ok(translatorBody([])),
      '/internal/process': () => ok(procBody),
    };
    install();
    component = mount(RuntimeCards, { target: document.body });
    await settle();
    expect(document.querySelector('[data-testid="card-sticky"]')).not.toBeNull();
    expect(document.querySelector('[data-testid="card-translator"]')).toBeNull();

    stickyBody = { enabled: false };
    sticky.refresh();
    await vi.waitFor(() => {
      flushSync();
      expect(document.querySelector('[data-testid="card-sticky"]')).toBeNull();
    });
    // ...and comes back with fresh values, not the old snapshot.
    stickyBody = { enabled: true, active_sessions: 7, hits: 0, misses: 0, max_sessions: 10 };
    sticky.refresh();
    await vi.waitFor(() => {
      flushSync();
      expect(document.querySelector('[data-testid="card-sticky"]')?.textContent).toContain('7');
    });
    expect(txt()).not.toContain('NaN');
  });

  it('shows no card, and no error panel, when an endpoint answers 403 on first load', async () => {
    routes = {
      '/internal/stats/sticky': () => fail(403),
      '/internal/stats/translators': () => fail(404),
      '/internal/process': () => fail(403),
    };
    install();
    component = mount(RuntimeCards, { target: document.body });
    await settle();
    expect(document.body.querySelector('.section')).toBeNull();
  });

  it('greys a card and adds a note when a later poll fails, then clears on recovery', async () => {
    let fail403 = false;
    routes = {
      '/internal/stats/sticky': () => ok({ enabled: false }),
      '/internal/stats/translators': () => ok(translatorBody([])),
      '/internal/process': () => (fail403 ? fail(503) : ok(procBody)),
    };
    install();
    component = mount(RuntimeCards, { target: document.body });
    await settle();
    fail403 = true;
    processStats.refresh();
    await vi.waitFor(() => {
      flushSync();
      expect(document.querySelector('[data-testid="card-process"] .tile-grid')?.getAttribute('data-state')).toBe('error');
    });
    expect(txt()).toContain('4.67 MB');
    expect(txt()).toContain('retrying');
    fail403 = false;
    processStats.refresh();
    await vi.waitFor(() => {
      flushSync();
      expect(document.querySelector('[data-testid="card-process"] .tile-grid')?.hasAttribute('data-state')).toBe(false);
    });
    expect(txt()).not.toContain('retrying');
  });

  it('renders a process payload with missing optional fields without throwing', async () => {
    routes = {
      '/internal/stats/sticky': () => ok({ enabled: false }),
      '/internal/stats/translators': () => ok(translatorBody([])),
      '/internal/process': () =>
        ok({ memory: {}, garbage_collection: {}, goroutines: { count: 5 }, runtime: { uptime: '1s' } }),
    };
    install();
    component = mount(RuntimeCards, { target: document.body });
    await settle();
    const card = document.querySelector('[data-testid="card-process"]')?.textContent ?? '';
    expect(card).toContain('Goroutines');
    expect(card).not.toContain('undefined');
  });

  it('renders a card per translator that has handled requests, and none for idle ones', async () => {
    routes = {
      '/internal/stats/sticky': () => ok({ enabled: false }),
      '/internal/stats/translators': () => ok(translatorBody([anth, { ...anth, translator_name: 'other', total_requests: 0 }])),
      '/internal/process': () => ok(procBody),
    };
    install();
    component = mount(RuntimeCards, { target: document.body });
    await settle();
    const cards = document.querySelectorAll('[data-testid="card-translator"]');
    expect(cards.length).toBe(1);
    expect(cards[0].textContent).toContain('50%');
    expect(txt()).not.toContain('other');
  });

  it('shows <0.1% of capacity for a tiny non-zero fill', async () => {
    routes = {
      '/internal/stats/sticky': () => ok({ enabled: true, active_sessions: 1, max_sessions: 100000, hits: 0, misses: 0 }),
      '/internal/stats/translators': () => ok(translatorBody([])),
      '/internal/process': () => ok(procBody),
    };
    install();
    component = mount(RuntimeCards, { target: document.body });
    await settle();
    expect(txt()).toContain('<0.1% of capacity');
    expect(txt()).not.toContain('0.0%');
  });

  it('keeps the last good cards through a malformed payload, then takes the next valid one', async () => {
    let tBody: unknown = translatorBody([anth]);
    let pBody: unknown = procBody;
    let sBody: unknown = { enabled: true, active_sessions: 2, hits: 1, misses: 1, max_sessions: 10 };
    routes = {
      '/internal/stats/sticky': () => ok(sBody),
      '/internal/stats/translators': () => ok(tBody),
      '/internal/process': () => ok(pBody),
    };
    install();
    component = mount(RuntimeCards, { target: document.body });
    await settle();
    expect(document.querySelectorAll('[data-testid^="card-"]').length).toBe(3);

    for (const [t, p] of [
      [{ translators: [null] }, { ...procBody, memory: { ...procBody.memory, memory_pressure: 7 } }],
      [translatorBody([{ ...anth, total_requests: '10' }]), procBody],
    ] as const) {
      tBody = t;
      pBody = p;
      sBody = {};
      await settle2();
      // Malformed bodies are rejected: the snapshots are unchanged and every poll shows as failed.
      expect(document.querySelectorAll('[data-testid^="card-"]').length).toBe(3);
      expect(translators.status).toBe('error');
      expect(sticky.status).toBe('error');
      expect(txt()).toContain('4.67 MB');
      expect(txt()).toContain('medium');
      expect(txt()).toContain('Translator: anthropic');
      expect(txt().replace(/refresh failed, retrying|·/g, '')).toContain('4 requests');
    }

    tBody = translatorBody([null, { ...anth, translator_name: 'second' }]);
    pBody = procBody;
    sBody = { enabled: true, active_sessions: 9, max_sessions: 10 };
    await settle2();
    expect(translators.status).toBe('ok');
    expect(txt()).toContain('second');
    expect(txt()).not.toContain('Translator: anthropic');
  });
});
