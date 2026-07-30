import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { endpoints } from '../lib/stores/endpoints.svelte.js';
import EndpointsPanel from './EndpointsPanel.svelte';

// C7 regression: PctBar derived its colour purely from success_rate_num and
// request_count, with no reference to endpoint status. An OFFLINE endpoint
// with lifetime counters therefore rendered a full green success bar beside
// its red offline pill - confidently advertising 100% success for a backend
// that is down. The fix gates the bar's COLOUR on status: an offline endpoint
// with history renders the neutral bucket, not green.
//
// A follow-up regression: the first attempt at that fix gated the FIGURE
// away too (hasData && !down), so an offline endpoint with 10,000 requests
// and an endpoint that never took a single request both rendered "no data" -
// pixel-identical, throwing away real history exactly when an operator needs
// it. The fix distinguishes "no data" (never traded) from "historical"
// (down, but we have a real figure): the latter keeps the number and real
// fill width, muted, with no green/amber/red implication.

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
  it('mutes an offline endpoint with traffic but keeps its historical figure', async () => {
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
    // Not the green bucket - a dead backend must not paint confident health.
    expect(fill.getAttribute('style')).not.toContain('green');
    // But the real figure survives: this endpoint has history, unlike one
    // that never took a request, and hiding it destroys that distinction.
    expect(fill.getAttribute('style')).toMatch(/width:\s*100%/);
    expect(pctCell().textContent).toContain('100%');
    expect(pctCell().textContent).not.toContain('no data');
  });

  it('shows no data for an offline endpoint that never took a request', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    await refreshWith({
      name: 'ollama-never-traded',
      url: 'http://ollama-never-traded:11434',
      type: 'ollama',
      status: 'offline',
      priority: 100,
      success_rate: '0.0%',
      request_count: 0,
      model_count: 1,
      avg_latency_ms: null,
      min_latency_ms: 0,
      max_latency_ms: 0,
    });

    const fill = pctFill();
    expect(fill).toBeTruthy();
    expect(fill.getAttribute('style')).not.toContain('green');
    // No history at all - "no data" is still the right call here.
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
