import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { endpoints } from '../lib/stores/endpoints.svelte.js';
import { models } from '../lib/stores/models.svelte.js';
import EndpointsPanel from './EndpointsPanel.svelte';
import ModelsPanel from './ModelsPanel.svelte';

// C10 regression: `td.num { text-align: right }` only positions INLINE content.
// The Success (PctBar), Latency (RangeBar) and Endpoints (chips) cells contain
// block/flex children that ignore text-align, so the header right-aligned
// while the cell content stayed hard left - a wide dead gap. The fix
// separates `num` (sort semantics) from alignment (presentation): composite
// cells carry an explicit `align-right` marker whose CSS pushes their flex
// children right via auto margin. jsdom does not lay out flex, so this asserts
// the structural decision; visual confirmation is deferred to the browser
// pass (WP6).
//
// Follow-up (post-remediation review): `column.align` was originally added to
// the column definitions but never read anywhere - the `align-right` class on
// each <td> was a hand-authored literal, so the field was documentation-only
// and a future composite column could still be added without the guard doing
// anything. `SortableTable` now derives both the header's and the cell's
// class from `column.align` via a `cellClass` helper handed to the row/group
// snippets, so panels no longer author "num align-right" literals at all -
// asserted below via the header class alongside the cell class.

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

describe('composite numeric cells carry an explicit alignment class', () => {
  it('marks the Success and Latency cells align-right in EndpointsPanel', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    global.fetch = vi.fn(async () =>
      jsonResponse({
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
      })
    );
    endpoints.refresh();
    await vi.waitFor(() => expect(endpoints.data?.endpoints?.[0]?.name).toBe('ollama-1'));
    flushSync();

    const row = document.querySelector('tbody tr');
    const tds = [...row.querySelectorAll('td')];

    const successCell = tds.find((td) => td.querySelector('.pct-cell'));
    const latencyCell = tds.find((td) => td.querySelector('.range-wrap'));
    expect(successCell).toBeTruthy();
    expect(latencyCell).toBeTruthy();

    // Composite cells: alignment is explicit, decoupled from the `num` sort
    // flag. The flex-child CSS rule keys off this class.
    expect(successCell.classList.contains('align-right')).toBe(true);
    expect(latencyCell.classList.contains('align-right')).toBe(true);
    // They remain `num` for sort semantics.
    expect(successCell.classList.contains('num')).toBe(true);

    // The header carries the same `align-right` class, driven by the same
    // column.align field the cell class came from - not a separately
    // hand-maintained header class.
    const headers = [...document.querySelectorAll('thead th')];
    const successHeader = headers.find((th) => th.textContent.includes('Success'));
    const latencyHeader = headers.find((th) => th.textContent.includes('Latency'));
    expect(successHeader.classList.contains('align-right')).toBe(true);
    expect(latencyHeader.classList.contains('align-right')).toBe(true);

    // A genuinely numeric text column (Priority) is also driven by
    // column.align now, not by `num` alone - proving `num` no longer does
    // double duty as an alignment flag anywhere in the table.
    const priorityHeader = headers.find((th) => th.textContent.includes('Priority'));
    expect(priorityHeader.classList.contains('align-right')).toBe(true);
    const priorityCell = tds[headers.indexOf(priorityHeader)];
    expect(priorityCell.classList.contains('align-right')).toBe(true);
  });

  it('marks the Endpoints (chips) cell align-right in ModelsPanel', async () => {
    component = mount(ModelsPanel, { target: document.body });
    flushSync();

    global.fetch = vi.fn(async () =>
      jsonResponse({
        model_groups: [
          {
            family: 'qwen',
            model_count: 1,
            endpoints: ['ollama-1', 'vllm-1'],
            models: [
              {
                name: 'qwen3:8b',
                params: '8B',
                quant: 'q4_k_m',
                size: '4.5 GB',
                endpoints: ['ollama-1', 'vllm-1'],
                last_seen_at: new Date().toISOString(),
              },
            ],
          },
        ],
      })
    );
    models.refresh();
    await vi.waitFor(() => expect(models.data?.model_groups?.length).toBe(1));
    flushSync();

    const row = [...document.querySelectorAll('tbody tr')].find((tr) =>
      tr.querySelector('.endpoint-pills')
    );
    expect(row).toBeTruthy();
    const chipsCell = [...row.querySelectorAll('td')].find((td) =>
      td.querySelector('.endpoint-pills')
    );
    expect(chipsCell.classList.contains('align-right')).toBe(true);

    // Same guard as EndpointsPanel: the Endpoints column's header carries
    // the matching align-right class, both driven by the one `align: 'right'`
    // on the column definition rather than two things a caller must keep in
    // sync by hand.
    const endpointsHeader = [...document.querySelectorAll('thead th')].find((th) =>
      th.textContent.includes('Endpoints')
    );
    expect(endpointsHeader.classList.contains('align-right')).toBe(true);
  });
});
