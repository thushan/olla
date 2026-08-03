import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, beforeAll, vi } from 'vitest';
import App from './App.svelte';
import { navigation } from './lib/stores/navigation.svelte';
import { overview } from './lib/stores/overview.svelte';
import { endpoints } from './lib/stores/endpoints.svelte';
import { stableId } from './lib/dom-id';

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

let component: ReturnType<typeof mount> | undefined;
afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
  navigation.set('overview');
});

function jsonResponse(body: unknown) {
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
            url: 'http://ollama-1:11434',
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

    const jumpBtn = document.querySelector<HTMLElement>('.glance-link');
    expect(jumpBtn).toBeTruthy();
    jumpBtn!.click();

    const row = await vi.waitFor(() => {
      const el = document.getElementById(`ep-${stableId('http://ollama-1:11434')}`);
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

  // Regression coverage for the DOM-id collision bug. The old cssId() slug
  // stripped punctuation, so "node.a" and "node-a" both produced id
  // "ep-node-a". getElementById always returned whichever row rendered first
  // in DOM order - here node.a (priority 100 sorts first). Clicking "jump"
  // from the node-a glance row therefore scrolled to and focused node.a's row,
  // silently wrong, with no error. The prior test deliberately jumped from
  // node.a (which self-resolves) to avoid encoding the bug as a passing spec;
  // this case flips that and jumps from node-a, asserting it lands on node-a.
  it('jumps from node-a to node-a (not node.a) when names collide once slugged', async () => {
    component = mount(App, { target: document.body });
    flushSync();

    global.fetch = vi.fn(async (url) => {
      if (String(url).includes('/internal/status/endpoints')) {
        return jsonResponse({
          endpoints: [
            {
              name: 'node.a',
              url: 'http://node-a:11434',
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
            {
              name: 'node-a',
              url: 'http://node-b:11434',
              type: 'ollama',
              status: 'healthy',
              priority: 90,
              success_rate: '98.0%',
              request_count: 8,
              model_count: 1,
              avg_latency_ms: 25,
              min_latency_ms: 5,
              max_latency_ms: 40,
            },
          ],
          total_count: 2,
          healthy_count: 2,
          routable_count: 2,
        });
      }
      return jsonResponse(sysBody);
    });
    overview.refresh();
    endpoints.refresh();
    await vi.waitFor(() => {
      expect(overview.data?.system?.status).toBe('healthy');
      expect(endpoints.data?.endpoints?.length).toBe(2);
    });
    flushSync();

    // Both glance rows rendered - the panel did not blank on the collision.
    const glanceLinks = [...document.querySelectorAll<HTMLElement>('.glance-link .txt')].map((el) =>
      el.textContent!.trim()
    );
    expect(glanceLinks).toEqual(expect.arrayContaining(['node.a', 'node-a']));

    // The two endpoint rows carry DISTINCT DOM ids (the old code gave both
    // ep-node-a).
    const nodeAUrl = 'http://node-a:11434';
    const nodeBUrl = 'http://node-b:11434';
    const idA = `ep-${stableId(nodeAUrl)}`;
    const idB = `ep-${stableId(nodeBUrl)}`;
    expect(idA).not.toBe(idB);

    // Jump from node-a's glance row.
    const jumpBtn = [...document.querySelectorAll<HTMLElement>('.glance-link')].find(
      (btn) => btn.querySelector('.txt')?.textContent?.trim() === 'node-a'
    );
    expect(jumpBtn).toBeTruthy();
    jumpBtn!.click();

    const row = await vi.waitFor(() => {
      const el = document.getElementById(idB);
      expect(el).toBeTruthy();
      return el;
    });

    expect(document.getElementById('panel-endpoints')).not.toBeNull();
    expect(document.getElementById('panel-overview')).toBeNull();
    expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: 'center' });
    expect(document.activeElement).toBe(row);
    expect(document.activeElement).not.toBe(document.body);
    // Landed on node-a's row, NOT node.a's. Assert against the name cell's
    // exact text, not loose textContent: the URL "http://node-a:..." contains
    // the substring "node-a", which would mask the wrong-target failure.
    const landedName = row!.querySelector('.name-text')?.textContent?.trim();
    expect(landedName).toBe('node-a');
  });
});
