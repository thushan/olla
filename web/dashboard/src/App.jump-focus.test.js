import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, beforeAll, vi } from 'vitest';
import App from './App.svelte';
import { navigation } from './lib/stores/navigation.svelte.js';
import { overview } from './lib/stores/overview.svelte.js';
import { endpoints } from './lib/stores/endpoints.svelte.js';

// Regression coverage: "jump to endpoint" on the Overview glance table looked
// up `ep-<slug>` in the DOM, but the generic row path in SortableTable never
// set an `id` on its <tr>, so the lookup always returned null. scrollIntoView
// never ran, and since the panel swap unmounts OverviewPanel entirely
// (App.svelte's {#if}), the clicked button was destroyed and keyboard focus
// fell through to <body> with nothing to replace it.

// jsdom doesn't implement scrollIntoView; stub it so the fixed code doesn't
// throw and so the test can assert it was actually called.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

let component;
afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
  navigation.set('overview');
});

function jsonResponse(body) {
  return {
    status: 200,
    ok: true,
    headers: { get: () => null },
    json: async () => body,
  };
}

const sysBody = {
  system: {
    status: 'healthy',
    endpoints_up: '1/1',
    success_rate: '99.0%',
    avg_latency: '20ms',
    total_traffic: '1.00 GB',
    active_connections: 0,
    total_requests: 10,
    total_failures: 0,
    security_violations: 0,
    version: 'v0.0.29',
    commit: 'abc123',
    start_time: new Date().toISOString(),
  },
  proxy: { engine: 'olla', balancer: 'priority' },
};

async function seedData() {
  global.fetch = vi.fn(async (url) => {
    if (String(url).includes('/internal/status/endpoints')) {
      return jsonResponse({
        endpoints: [
          {
            name: 'ollama-1',
            type: 'ollama',
            status: 'healthy',
            priority: 100,
            success_rate: '99.0%',
            request_count: 10,
            model_count: 1,
            avg_latency_ms: 20,
            min_latency_ms: 5,
            max_latency_ms: 40,
          },
        ],
        total_count: 1,
        healthy_count: 1,
        routable_count: 1,
      });
    }
    return jsonResponse(sysBody);
  });
  overview.refresh();
  endpoints.refresh();
  await vi.waitFor(() => {
    expect(overview.data?.system?.status).toBe('healthy');
    expect(endpoints.data?.endpoints?.[0]?.name).toBe('ollama-1');
  });
  flushSync();
}

describe('Overview "jump to endpoint" scroll and focus', () => {
  it('scrolls to and focuses the target row after the panel swap commits', async () => {
    component = mount(App, { target: document.body });
    flushSync();

    await seedData();

    const jumpBtn = document.querySelector('.glance-link');
    expect(jumpBtn).toBeTruthy();
    jumpBtn.click();

    const row = await vi.waitFor(() => {
      const el = document.getElementById('ep-ollama-1');
      expect(el).toBeTruthy();
      return el;
    });

    // The panel actually swapped.
    expect(document.getElementById('panel-endpoints')).not.toBeNull();
    expect(document.getElementById('panel-overview')).toBeNull();

    expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: 'center' });

    // Focus landed on the row, not the document root left behind by the
    // destroyed button.
    expect(document.activeElement).toBe(row);
    expect(document.activeElement).not.toBe(document.body);
  });
});
