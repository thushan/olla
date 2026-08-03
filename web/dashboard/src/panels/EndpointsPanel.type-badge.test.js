import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { endpoints } from '../lib/stores/endpoints.svelte.ts';
import EndpointsPanel from './EndpointsPanel.svelte';

// C8 regression: the Endpoints name cell rendered an inline badge-type span
// showing e.type AND a separate Type column also showing e.type - the same
// value twice per row. The fix drops the inline badge (the Type column is the
// canonical place for it).

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
      healthy_count: 1,
      routable_count: 1,
    })
  );
  endpoints.refresh();
  await vi.waitFor(() => expect(endpoints.data?.endpoints?.[0]?.name).toBe(endpoint.name));
  flushSync();
}

describe('EndpointsPanel type is shown exactly once per row', () => {
  it('renders no inline badge-type span in the name cell', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    await refreshWith({
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
    });

    // The inline badge is gone.
    expect(document.querySelector('.badge-type')).toBeNull();

    // The Type column still carries the value exactly once in the row's cells.
    const row = document.querySelector('tbody tr');
    const typeMentions = [...row.querySelectorAll('td')].filter((td) =>
      td.textContent.trim() === 'ollama'
    );
    expect(typeMentions.length).toBe(1);
  });
});
