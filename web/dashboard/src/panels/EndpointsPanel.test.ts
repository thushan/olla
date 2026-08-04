import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { endpoints } from '../lib/stores/endpoints.svelte';
import EndpointsPanel from './EndpointsPanel.svelte';

// Regression coverage for finding 6: the panel used to derive the RangeBar's
// average from `response_time` (parseInt on the last HEALTH-CHECK probe's
// formatted latency string) instead of the API's own `avg_latency_ms`
// (average PROXY request latency - a different metric entirely). A 1500ms
// health check formatted as "1.5s" and parseInt("1.5s", 10) silently
// produced 1, so the bar rendered a 1ms average for an endpoint that took
// 1.5 SECONDS to answer its last health check.
//
// Note: deliberately static imports, no vi.resetModules() - the `endpoints`
// store is a module singleton, and resetting modules mid-file desyncs
// Svelte's internal client runtime from the already-mounted component.

let component: ReturnType<typeof mount> | undefined;
afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
});

function jsonResponse(body: unknown) {
  return {
    status: 200,
    ok: true,
    headers: { get: () => null },
    json: async () => body,
  };
}

async function refreshWithEndpoint(endpoint: Record<string, unknown>) {
  global.fetch = vi.fn(async () =>
    jsonResponse({ endpoints: [endpoint], total_count: 1, healthy_count: 1, routable_count: 1 })
  );
  endpoints.refresh();
  // Wait for this specific endpoint's payload, not just status === 'ok' -
  // the store is a module singleton reused across tests, so status may
  // already read 'ok' from a previous test before this refresh resolves.
  await vi.waitFor(() => expect(endpoints.data?.endpoints?.[0]?.name).toBe(endpoint.name));
  flushSync();
}

async function refreshWithEndpoints(endpointList: Record<string, unknown>[]) {
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

describe('EndpointsPanel latency column', () => {
  it('uses avg_latency_ms, not a parse of response_time, for the RangeBar average', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    await refreshWithEndpoint({
      name: 'ollama-1',
      type: 'ollama',
      status: 'healthy',
      priority: 100,
      success_rate: '99.0%',
      request_count: 40,
      model_count: 1,
      // Last health-check probe took 1.5s - an unrelated metric that must
      // never leak into the proxy-latency average.
      response_time: '1.5s',
      avg_latency_ms: 42,
      min_latency_ms: 10,
      max_latency_ms: 90,
    });

    const label = document.querySelector('.range-bar[role="img"]')!.getAttribute('aria-label');
    expect(label).toContain('average 42ms');
    // The old bug rendered "1ms" here (parseInt("1.5s", 10)).
    expect(label).not.toContain('average 1ms');
    expect(document.querySelector('.range-labels')!.firstChild!.textContent!.trim()).toBe('42ms');
  });

  it('falls back to the no-data placeholder when avg_latency_ms is absent (backend not upgraded yet)', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    await refreshWithEndpoint({
      name: 'ollama-2',
      type: 'ollama',
      status: 'healthy',
      priority: 100,
      success_rate: '99.0%',
      request_count: 40,
      model_count: 1,
      response_time: '1.5s',
      // no avg_latency_ms field at all
      min_latency_ms: 10,
      max_latency_ms: 90,
    });

    const avgText = document.querySelector('.range-labels')!.firstChild!.textContent!.trim();
    expect(avgText).toBe('—');
    // Must not have silently coerced the missing field, or response_time, to 0/1.
    expect(avgText).not.toBe('0ms');
    expect(avgText).not.toBe('1ms');
  });

  // Regression test for the globalLatencyMax/globalMax wiring bug: the panel
  // passed `{globalLatencyMax}` (shorthand -> prop named globalLatencyMax) but
  // RangeBar destructures `globalMax`, so the prop was always its default 0
  // and every bar rendered at 0% width with its peak tick collapsed to the
  // left edge, table-wide, while the text labels kept reading correctly.
  // RangeBar.test.js alone can't catch this because it passes the
  // correctly-named prop directly - the mistake only exists at the call site
  // in this file, so the test has to go through EndpointsPanel. Two
  // endpoints with different maxima so a genuinely-wired globalMax produces
  // distinct, non-zero, non-100% geometry - a false pass isn't available by
  // coincidence.
  it('wires globalLatencyMax into RangeBar as globalMax so bars render non-zero geometry', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    await refreshWithEndpoints([
      {
        name: 'ollama-low',
        type: 'ollama',
        status: 'healthy',
        priority: 100,
        success_rate: '99.0%',
        request_count: 40,
        model_count: 1,
        avg_latency_ms: 100,
        min_latency_ms: 10,
        max_latency_ms: 200,
      },
      {
        name: 'ollama-high',
        type: 'ollama',
        status: 'healthy',
        priority: 90,
        success_rate: '99.0%',
        request_count: 40,
        model_count: 1,
        avg_latency_ms: 50,
        min_latency_ms: 5,
        max_latency_ms: 800,
      },
    ]);

    // globalLatencyMax across both rows is 800 (the second endpoint's max).
    const bars = [...document.querySelectorAll('.range-bar[role="img"]')];
    const lowBar = bars.find((b) => b.getAttribute('aria-label')!.includes('average 100ms'))!;
    const highBar = bars.find((b) => b.getAttribute('aria-label')!.includes('average 50ms'))!;
    expect(lowBar).toBeTruthy();
    expect(highBar).toBeTruthy();

    // scalePct(avg, 800) = sqrt(avg/800)*100. With the pre-fix binding
    // globalMax defaults to 0 and both of these collapse to "0.0".
    const lowFill = lowBar.querySelector('.fill')!.getAttribute('style');
    const lowTick = lowBar.querySelector('.tick')!.getAttribute('style');
    const highFill = highBar.querySelector('.fill')!.getAttribute('style');
    const highTick = highBar.querySelector('.tick')!.getAttribute('style');

    expect(lowFill).toBe('width: 35.4%;');
    expect(lowTick).toBe('left: 50%;');
    expect(highFill).toBe('width: 25%;');
    expect(highTick).toBe('left: 100%;');
  });
});

describe('EndpointsPanel banner isolation from panel dim', () => {
  // Structural regression, mirrors OverviewPanel's equivalent: opacity/filter
  // on an ancestor compound down the subtree, so a CSS override on .banner is
  // a no-op. The banner must live OUTSIDE the [data-state]-bearing wrapper to
  // stay at full contrast during an outage. Assert the DOM structure, not
  // computed style - jsdom has no layout.
  it('renders the StatusBanner as a sibling of, not inside, the data-state wrapper', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    await refreshWithEndpoint({ name: 'ep', type: 'ollama', status: 'healthy', priority: 100 });

    // Force the store into error status so the wrapper carries data-state='error'.
    global.fetch = vi.fn(async () => ({ status: 500, ok: false, headers: { get: () => null } }));
    endpoints.refresh();
    await vi.waitFor(() => expect(endpoints.status === 'error' || endpoints.status === 'stale').toBe(true));
    flushSync();

    const panel = document.getElementById('panel-endpoints')!;
    expect(panel).toBeTruthy();
    const dimmed = panel.querySelector('[data-state="stale"], [data-state="error"]');
    expect(dimmed).toBeTruthy();

    const banner = panel.querySelector('.banner');
    expect(banner).toBeTruthy();
    // The banner must NOT be inside the dimmed wrapper - it should be a
    // sibling of it within the panel root.
    expect(dimmed!.contains(banner)).toBe(false);
    expect(banner!.parentElement).toBe(panel);
  });
});
