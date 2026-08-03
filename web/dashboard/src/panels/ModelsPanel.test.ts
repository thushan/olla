import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { models } from '../lib/stores/models.svelte';
import { stableId } from '../lib/dom-id';
import ModelsPanel from './ModelsPanel.svelte';

// Coverage: the Models panel was trimmed to discovery-only columns
// (name, params, quant, size, endpoints, last seen). Per-model traffic
// columns and the per_endpoint tooltip lookup were removed because nothing
// on the proxy path populates those figures and per_endpoint has no prior
// art. These tests pin the trim so a future revert is caught.

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

async function refreshWith(payload: Record<string, unknown>, ready: () => void) {
  global.fetch = vi.fn(async () => jsonResponse(payload));
  models.refresh();
  // Wait for the specific payload via a caller-supplied sentinel rather
  // than status alone: the store is a module singleton reused across
  // tests, so status may already read 'ok' from a previous test before
  // this refresh resolves.
  await vi.waitFor(() => ready());
  flushSync();
}

const groupedPayload = {
  model_groups: [
    {
      family: 'qwen',
      model_count: 2,
      endpoints: ['ollama-1'],
      models: [
        {
          name: 'qwen3:8b',
          params: '8B',
          quant: 'q4_k_m',
          size: '4.5 GB',
          endpoints: ['ollama-1'],
          last_seen_at: new Date().toISOString(),
          // Fields the trim removed must be ignored if a legacy backend
          // still sends them. Include them to prove the panel no longer
          // renders them regardless of wire shape.
          per_endpoint: [{ endpoint: 'ollama-1', parameter_size: '8B', size_bytes: 4831838208 }],
          total_requests: 999,
          success_rate: '99.0%',
          p95_latency: '120ms',
          p99_latency: '200ms',
        },
        {
          name: 'qwen3:14b',
          params: '14B',
          quant: 'q5_k_m',
          size: '9.0 GB',
          endpoints: ['ollama-1', 'vllm-1'],
          last_seen_at: new Date().toISOString(),
        },
      ],
    },
    {
      family: 'unknown',
      model_count: 1,
      endpoints: ['llamacpp-1'],
      models: [
        {
          name: 'mystery-model',
          params: '7B',
          quant: 'q8_0',
          size: '7.0 GB',
          endpoints: ['llamacpp-1'],
          last_seen_at: new Date().toISOString(),
        },
      ],
    },
  ],
};

function headerLabels() {
  // SortableTable renders the indicator glyph (↕/▲/▼) inside the button;
  // strip it so label assertions compare against the bare column label.
  return [...document.querySelectorAll('thead th')].map((th) =>
    th.textContent!.trim().replace(/[↕▲▼]/g, '').trim()
  );
}

describe('ModelsPanel trim (discovery-only)', () => {
  it('renders only discovery columns; no traffic columns', async () => {
    component = mount(ModelsPanel, { target: document.body });
    flushSync();

    await refreshWith(groupedPayload, () =>
      expect(models.data?.model_groups?.length).toBe(groupedPayload.model_groups.length)
    );

    const labels = headerLabels();
    for (const label of ['Model', 'Params', 'Quant', 'Size', 'Endpoints', 'Last seen']) {
      expect(labels).toContain(label);
    }
    // Traffic columns removed during the discovery-only trim must not come back.
    for (const label of ['Requests', 'Success', 'p95', 'p99']) {
      expect(labels).not.toContain(label);
    }
  });

  it('does not render per-model traffic cells even if the payload carries them', async () => {
    component = mount(ModelsPanel, { target: document.body });
    flushSync();

    await refreshWith(groupedPayload, () =>
      expect(models.data?.model_groups?.length).toBe(groupedPayload.model_groups.length)
    );

    const qwenRow = document.getElementById(`model-${stableId('qwen3:8b')}`);
    expect(qwenRow).toBeTruthy();
    const cells = [...qwenRow!.querySelectorAll('td')].map((td) => td.textContent!.trim());
    // The legacy total_requests value must not surface anywhere in the row.
    expect(cells.some((c) => c.includes('999'))).toBe(false);
    expect(cells.some((c) => c.includes('99.0%'))).toBe(false);
    expect(cells.some((c) => c.includes('120ms'))).toBe(false);
    expect(cells.some((c) => c.includes('200ms'))).toBe(false);
  });

  it('removes the per_endpoint tooltip from endpoint pills', async () => {
    component = mount(ModelsPanel, { target: document.body });
    flushSync();

    await refreshWith(groupedPayload, () =>
      expect(models.data?.model_groups?.length).toBe(groupedPayload.model_groups.length)
    );

    const qwenRow = document.getElementById(`model-${stableId('qwen3:8b')}`);
    const pills = [...qwenRow!.querySelectorAll('.pill')];
    expect(pills.length).toBe(1);
    // per_endpoint.parameter_size was '8B'; the trimmed pill must not leak it.
    expect(pills[0].getAttribute('title') || '').toBe('');
  });

  it('renders family group headers and sorts unknown last', async () => {
    component = mount(ModelsPanel, { target: document.body });
    flushSync();

    await refreshWith(groupedPayload, () =>
      expect(models.data?.model_groups?.length).toBe(groupedPayload.model_groups.length)
    );

    const familyRows = [...document.querySelectorAll('.family-row')].map((tr) =>
      tr.textContent!.trim()
    );
    expect(familyRows.length).toBe(2);
    // Alphabetical, with 'unknown' pushed to the end (matches backend ordering).
    expect(familyRows[0]).toContain('qwen');
    expect(familyRows[1]).toContain('unknown');
  });

  it('falls back to the flat recent_models shape when no groups are present', async () => {
    component = mount(ModelsPanel, { target: document.body });
    flushSync();

    await refreshWith(
      {
        recent_models: [
          {
            name: 'phi3',
            params: '3B',
            quant: 'q4',
            size: '2.0 GB',
            endpoints: ['ollama-2'],
            last_seen_at: new Date().toISOString(),
          },
        ],
      },
      () => expect(models.data?.recent_models?.[0]?.name).toBe('phi3')
    );

    // Flat path renders rows directly (no family-row grouping). The flat
    // SortableTable path does not plant a DOM id on rows (only the grouped
    // path's snippet does); locate the row by its model name instead.
    expect(document.querySelectorAll('.family-row').length).toBe(0);
    const rowText = [...document.querySelectorAll('tbody td')].map((td) => td.textContent!.trim());
    expect(rowText.some((t) => t === 'phi3')).toBe(true);
    // Discovery column still present in the header.
    expect(headerLabels()).toContain('Size');
    expect(headerLabels()).not.toContain('p95');
  });
});
