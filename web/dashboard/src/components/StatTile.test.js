import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { resolve, dirname } from 'node:path';
import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { overview } from '../lib/stores/overview.svelte.js';
import { endpoints } from '../lib/stores/endpoints.svelte.js';
import OverviewPanel from '../panels/OverviewPanel.svelte';

// C4 regression: StatTile exposed valueHtml/subHtml props that bypassed
// Svelte's escaping via {@html}. Inert today (only closed server-controlled
// enums/integers flow through them) but an opt-in XSS footgun on a shared
// component. The fix expresses the same rich markup through Svelte snippets,
// which compose into the DOM as normal content (escaped, no raw HTML sink).

const here = dirname(fileURLToPath(import.meta.url));
const statTileSrc = readFileSync(resolve(here, './StatTile.svelte'), 'utf8');

let component;
afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
});

describe('StatTile has no {@html} sink', () => {
  // Fail-first: the current StatTile source carries {@html} and the
  // valueHtml/subHtml props, so this assertion fails until the component is
  // migrated to snippets.
  it('source contains no {@html} tags and no valueHtml/subHtml props', () => {
    expect(statTileSrc).not.toMatch(/\{@html/);
    expect(statTileSrc).not.toMatch(/valueHtml/);
    expect(statTileSrc).not.toMatch(/subHtml/);
  });

  // Migration guard: the two Overview tiles that previously used {@html} still
  // render the same composed markup (glyph + status, unit span, emphasised
  // sub) once they move to snippets. Not a bug-catcher; guards the API change.
  it('renders glyph/unit/emphasis markup at the Overview call sites after migration', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    const sysBody = {
      system: {
        status: 'degraded',
        endpoints_up: '1/2',
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
    global.fetch = vi.fn(async (url) => {
      if (String(url).includes('/internal/status/endpoints')) {
        return {
          status: 200,
          ok: true,
          headers: { get: () => null },
          json: async () => ({ endpoints: [], total_count: 0, healthy_count: 0, routable_count: 0 }),
        };
      }
      return { status: 200, ok: true, headers: { get: () => null }, json: async () => sysBody };
    });
    overview.refresh();
    endpoints.refresh();
    await vi.waitFor(() => expect(overview.data?.system?.status).toBe('degraded'));
    flushSync();

    const tiles = [...document.querySelectorAll('.tile')];

    const statusTile = tiles.find((t) => t.querySelector('.label')?.textContent === 'System status');
    expect(statusTile).toBeTruthy();
    const statusValue = statusTile.querySelector('.value');
    expect(statusValue.querySelector('.glyph')).toBeTruthy();
    expect(statusValue.textContent).toContain('degraded');
    const statusSub = statusTile.querySelector('.sub');
    expect(statusSub.querySelector('strong')).toBeTruthy();
    expect(statusSub.textContent).toContain('olla');

    const epTile = tiles.find((t) => t.querySelector('.label')?.textContent === 'Endpoints up');
    expect(epTile.querySelector('.value .unit')).toBeTruthy();
    expect(epTile.querySelector('.value').textContent.replace(/\s+/g, ' ').trim()).toBe('1/ 2');
  });
});
