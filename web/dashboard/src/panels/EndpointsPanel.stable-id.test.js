import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { endpoints } from '../lib/stores/endpoints.svelte.js';
import EndpointsPanel from './EndpointsPanel.svelte';

// Regression coverage: sanitiseDisplayURL (server-side) strips query and
// fragment before `url` reaches the frontend, so two distinct endpoints that
// differ only by query string (e.g. two API keys against the same host)
// arrive with an IDENTICAL `url` field. The panel used to key rows on
// `row.url ?? row.name`, so keying on url alone would collide these two rows
// under the same each-key, exactly the each_key_duplicate failure mode the
// dup-names/keys regressions already cover for `name`.
//
// The backend now emits a stable `id` derived from the raw (pre-sanitisation)
// URL - see stableEndpointID in handler_status_endpoints.go - specifically so
// the frontend has an identity that survives sanitisation. This test proves
// the panel actually uses it: distinct rows, distinct DOM ids, for two
// endpoints sharing a display url.

let component;
afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
});

function jsonResponse(body) {
  return {
    status: 200,
    ok: true,
    headers: { get: () => null },
    json: async () => body,
  };
}

async function refreshWithEndpoints(endpointList) {
  global.fetch = vi.fn(async () =>
    jsonResponse({
      endpoints: endpointList,
      total_count: endpointList.length,
      healthy_count: endpointList.length,
      routable_count: endpointList.length,
    })
  );
  endpoints.refresh();
  const lastName = endpointList[endpointList.length - 1].name;
  await vi.waitFor(() =>
    expect(endpoints.data?.endpoints?.[endpointList.length - 1]?.name).toBe(lastName)
  );
  flushSync();
}

function endpointWith(id, name) {
  return {
    id,
    name,
    // Both endpoints sanitise down to the same display url - the whole
    // point of the regression.
    url: 'http://shared-host:11434',
    type: 'ollama',
    status: 'healthy',
    priority: 100,
    success_rate: '99.0%',
    request_count: 10,
    model_count: 1,
    avg_latency_ms: 20,
    min_latency_ms: 5,
    max_latency_ms: 40,
  };
}

describe('EndpointsPanel row identity uses the stable id, not the display url', () => {
  it('renders distinct rows and distinct DOM ids for endpoints sharing a display url', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    await refreshWithEndpoints([
      endpointWith('id-abc', 'shared-host-a'),
      endpointWith('id-xyz', 'shared-host-b'),
    ]);

    const nameCells = [...document.querySelectorAll('.name-text')].map((el) => el.textContent.trim());
    expect(nameCells).toEqual(expect.arrayContaining(['shared-host-a', 'shared-host-b']));
    expect(nameCells.length).toBe(2);

    // Distinct DOM ids - if both rows resolved to the same id, getElementById
    // (used by the Overview jump-to-endpoint feature) would only ever reach
    // one of them.
    const rowIds = [...document.querySelectorAll('tbody tr[id^="ep-"]')].map((el) => el.id);
    expect(new Set(rowIds).size).toBe(2);
  });

  it('falls back to url/name when id is absent (older backend)', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    const noIdEndpoint = endpointWith(undefined, 'legacy-endpoint');
    delete noIdEndpoint.id;

    await refreshWithEndpoints([noIdEndpoint]);

    const nameCells = [...document.querySelectorAll('.name-text')].map((el) => el.textContent.trim());
    expect(nameCells).toEqual(['legacy-endpoint']);
  });
});
