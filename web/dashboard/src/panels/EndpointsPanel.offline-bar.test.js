import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { endpoints } from '../lib/stores/endpoints.svelte.js';
import EndpointsPanel from './EndpointsPanel.svelte';

// C7 regression: PctBar derived its colour purely from success_rate_num and
// request_count, with no reference to endpoint status. An OFFLINE endpoint
// with lifetime counters therefore rendered a full green success bar beside
// its red offline pill - confidently advertising 100% success for a backend
// that is down. The fix gates the bar on status: when the endpoint is
// offline/unhealthy/critical the bar renders the neutral no-data state.

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

async function refreshWith(endpoint) {
  global.fetch = vi.fn(async () =>
    jsonResponse({
      endpoints: [endpoint],
      total_count: 1,
      healthy_count: 0,
      routable_count: 0,
    })
  );
  endpoints.refresh();
  await vi.waitFor(() => expect(endpoints.data?.endpoints?.[0]?.name).toBe(endpoint.name));
  flushSync();
}

const pctCell = () => document.querySelector('.pct-cell');
const pctFill = () => pctCell()?.querySelector('.fill');

describe('EndpointsPanel success bar is gated on endpoint status', () => {
  it('does not render a green success bar for an offline endpoint with traffic', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    await refreshWith({
      name: 'ollama-down',
      url: 'http://ollama-down:11434',
      type: 'ollama',
      status: 'offline',
      priority: 100,
      // Lifetime counters make the unguarded bar paint green at 100%.
      success_rate: '100.0%',
      request_count: 500,
      model_count: 1,
      avg_latency_ms: 20,
      min_latency_ms: 5,
      max_latency_ms: 40,
    });

    const fill = pctFill();
    expect(fill).toBeTruthy();
    // Not the green bucket.
    expect(fill.getAttribute('style')).not.toContain('green');
    // The cell announces no data rather than a confident success rate.
    expect(pctCell().textContent).toContain('no data');
  });

  it('renders the success bar for a healthy endpoint with traffic', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    await refreshWith({
      name: 'ollama-up',
      url: 'http://ollama-up:11434',
      type: 'ollama',
      status: 'healthy',
      priority: 100,
      success_rate: '99.0%',
      request_count: 500,
      model_count: 1,
      avg_latency_ms: 20,
      min_latency_ms: 5,
      max_latency_ms: 40,
    });

    const fill = pctFill();
    expect(fill.getAttribute('style')).toContain('green');
    expect(pctCell().textContent).toContain('99.0%');
  });
});
