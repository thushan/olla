import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { endpoints } from '../lib/stores/endpoints.svelte';
import EndpointsPanel from './EndpointsPanel.svelte';
import { endpointFilter } from '../lib/stores/endpoint-filter.svelte';
import { jumpToEndpointKey } from '../lib/jump-to-endpoint';
import { stableId } from '../lib/dom-id';

// The shared filter is wired into the Endpoints panel as well as Overview.

let component: ReturnType<typeof mount> | undefined;
afterEach(() => {
  endpointFilter.clear();
  if (component) unmount(component);
  document.body.innerHTML = '';
});

const eps = [
  { name: 'gpu-box', type: 'ollama', status: 'healthy', id: 'a', url: 'http://a', priority: 1, model_count: 1, request_count: 0, active_connections: 0, success_rate: 'N/A', health_check: 'ok', min_latency_ms: 0, max_latency_ms: 0 },
  { name: 'mac-mini', type: 'vllm', status: 'offline', id: 'b', url: 'http://b', priority: 1, model_count: 0, request_count: 0, active_connections: 0, success_rate: 'N/A', health_check: 'x', issues: 'unavailable', min_latency_ms: 0, max_latency_ms: 0 },
];

describe('EndpointsPanel filter', () => {
  it('filters rows and shows the empty state', async () => {
    global.fetch = vi.fn(async () => ({
      status: 200, ok: true, headers: { get: () => null },
      json: async () => ({ endpoints: eps, total_count: 2, healthy_count: 1, routable_count: 1 }),
    })) as unknown as typeof fetch;
    endpoints.refresh();
    await vi.waitFor(() => expect(endpoints.data?.endpoints?.length).toBe(2));
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();
    const rows = () => document.querySelectorAll('table tbody tr').length;
    expect(rows()).toBe(2);
    const input = document.querySelector<HTMLInputElement>('.filter-input')!;
    input.value = 'vllm';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    expect(rows()).toBe(1);
    input.value = 'zzz';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    expect(rows()).toBe(0);
    expect(document.querySelector('.herd-empty')).not.toBeNull();
    input.value = '';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    [...document.querySelectorAll<HTMLButtonElement>('.chip')].find((c) => c.textContent === 'Unhealthy')!.click();
    flushSync();
    expect(rows()).toBe(1);
  });

  it('a jump to a filtered-out endpoint clears the filter and reveals the row', async () => {
    Element.prototype.scrollIntoView = vi.fn();
    global.fetch = vi.fn(async () => ({
      status: 200, ok: true, headers: { get: () => null },
      json: async () => ({ endpoints: eps, total_count: 2, healthy_count: 1, routable_count: 1 }),
    })) as unknown as typeof fetch;
    endpoints.refresh();
    await vi.waitFor(() => expect(endpoints.data?.endpoints?.length).toBe(2));
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();
    endpointFilter.query = 'mac';
    flushSync();
    expect(document.getElementById('ep-' + stableId('a'))).toBeNull();
    await jumpToEndpointKey(stableId('a'));
    flushSync();
    expect(endpointFilter.query).toBe('');
    expect(document.getElementById('ep-' + stableId('a'))).not.toBeNull();
  });
});
