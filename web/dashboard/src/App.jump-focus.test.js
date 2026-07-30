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

  // Regression coverage for the OverviewPanel rowId fix: two endpoints whose
  // names collide once slugged ("node.a" / "node-a") used to throw
  // each_key_duplicate on the glance table itself, and with no error
  // boundary anywhere in the app that blanked the ENTIRE Overview panel -
  // stat tiles included - so the glance-link this test depends on never
  // rendered and the jump path was unreachable. This proves the fix holds
  // with the collision actually present, not just on a collision-free fleet.
  //
  // Note: this deliberately jumps from "node.a", not "node-a". EndpointsPanel
  // and OverviewPanel both still derive their DOM id from the same lossy
  // cssId() slug (`ep-node-a` / `glance-node-a` for BOTH names), so
  // document.getElementById always resolves to whichever row rendered first
  // in DOM order - here that happens to be node.a's own row, so this jump
  // self-resolves correctly. Jumping from "node-a" would land on node.a's
  // row instead, silently. That DOM-id collision is a distinct latent defect
  // from the each-key one fixed here and is out of scope for this fix; see
  // docs/spec/simple-dashboard-findings.md.
  it('still jumps and focuses correctly when the glance table has colliding endpoint names', async () => {
    component = mount(App, { target: document.body });
    flushSync();

    global.fetch = vi.fn(async (url) => {
      if (String(url).includes('/internal/status/endpoints')) {
        return jsonResponse({
          endpoints: [
            {
              name: 'node.a',
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
    const glanceLinks = [...document.querySelectorAll('.glance-link .txt')].map((el) =>
      el.textContent.trim()
    );
    expect(glanceLinks).toEqual(expect.arrayContaining(['node.a', 'node-a']));

    const jumpBtn = [...document.querySelectorAll('.glance-link')].find(
      (btn) => btn.querySelector('.txt')?.textContent.trim() === 'node.a'
    );
    expect(jumpBtn).toBeTruthy();
    jumpBtn.click();

    const row = await vi.waitFor(() => {
      const el = document.getElementById('ep-node-a');
      expect(el).toBeTruthy();
      return el;
    });

    expect(document.getElementById('panel-endpoints')).not.toBeNull();
    expect(document.getElementById('panel-overview')).toBeNull();
    expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: 'center' });
    expect(document.activeElement).toBe(row);
    expect(document.activeElement).not.toBe(document.body);
    // The row focus actually landed on is the correct one for this jump.
    expect(row.textContent).toContain('node.a');
  });
});
