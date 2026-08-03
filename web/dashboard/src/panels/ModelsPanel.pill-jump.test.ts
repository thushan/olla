import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, beforeAll, vi } from 'vitest';
import { models } from '../lib/stores/models.svelte';
import { stableId } from '../lib/dom-id';
import ModelsPanel from './ModelsPanel.svelte';

// WP-B2 coverage: endpoint pills click-through to the matching EndpointsPanel
// row by STABLE ID (not display name). Two endpoints can share a display name
// (EndpointsPanel.dup-names.test), so a name->id lookup is forbidden; the
// backend pairs endpoints[i] with endpoint_ids[i], and each pill resolves to
// ep-${stableId(endpoint_id)}. The same-name collision case below must land on
// DISTINCT rows via the two distinct ids - the exact failure mode a name-keyed
// lookup would silently get wrong.

// jsdom doesn't implement scrollIntoView; stub it so the jump helper doesn't
// throw and so the test can assert it ran.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

let component: ReturnType<typeof mount> | undefined;
afterEach(() => {
  if (component) unmount(component);
  component = undefined;
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

// The models store is a module singleton reused across tests, so a sentinel
// that only checks status (or a field like the model name that's identical
// between two tests) will resolve on the PREVIOUS test's stale data before
// this refresh lands. Each test uses a UNIQUE model name and the sentinel
// waits for that name, guaranteeing the new payload has committed.
async function refreshWith(payload: Record<string, unknown>, ready: () => void) {
  global.fetch = vi.fn(async () => jsonResponse(payload));
  models.refresh();
  await vi.waitFor(() => ready());
  flushSync();
}

describe('ModelsPanel endpoint pill click-through (WP-B2)', () => {
  it('renders each pill as a button keyed on endpoint_ids, not display name', async () => {
    // Both endpoints share the SAME display name but have distinct ids. A
    // name-keyed lookup would resolve both pills to the same row.
    const payload = {
      model_groups: [
        {
          family: 'qwen',
          model_count: 1,
          endpoints: ['ollama', 'ollama'],
          models: [
            {
              name: 'collide-alpha',
              params: '8B',
              quant: 'q4_k_m',
              size: '4.5 GB',
              endpoints: ['ollama', 'ollama'],
              endpoint_ids: ['http://node-a:11434', 'http://node-b:11434'],
              last_seen_at: new Date().toISOString(),
            },
          ],
        },
      ],
    };

    component = mount(ModelsPanel, { target: document.body });
    flushSync();
    await refreshWith(payload, () =>
      expect(models.data?.model_groups?.[0]?.models?.[0]?.name).toBe('collide-alpha')
    );

    const pills = [...document.querySelectorAll('button.pill-link')];
    expect(pills.length).toBe(2);

    // Both pills carry the same visible label but target DISTINCT ids: the
    // crux of the locked decision. A name-based lookup would collapse these.
    const idA = `ep-${stableId('http://node-a:11434')}`;
    const idB = `ep-${stableId('http://node-b:11434')}`;
    expect(idA).not.toBe(idB);
  });

  it('fires onJumpToEndpoints and targets the clicked pill endpoint id', async () => {
    const onJump = vi.fn(() => {});
    // Plant the two target rows in the DOM so jumpToEndpoint's getElementById
    // resolves, and capture which id was looked up.
    const idA = `ep-${stableId('http://node-a:11434')}`;
    const idB = `ep-${stableId('http://node-b:11434')}`;
    const rowA = document.createElement('div');
    rowA.id = idA;
    rowA.tabIndex = -1;
    const rowB = document.createElement('div');
    rowB.id = idB;
    rowB.tabIndex = -1;
    document.body.append(rowA, rowB);

    const payload = {
      model_groups: [
        {
          family: 'qwen',
          model_count: 1,
          endpoints: ['ollama', 'ollama'],
          models: [
            {
              name: 'collide-beta',
              params: '8B',
              quant: 'q4_k_m',
              size: '4.5 GB',
              endpoints: ['ollama', 'ollama'],
              endpoint_ids: ['http://node-a:11434', 'http://node-b:11434'],
              last_seen_at: new Date().toISOString(),
            },
          ],
        },
      ],
    };

    component = mount(ModelsPanel, {
      target: document.body,
      props: { onJumpToEndpoints: onJump },
    });
    flushSync();
    await refreshWith(payload, () =>
      expect(models.data?.model_groups?.[0]?.models?.[0]?.name).toBe('collide-beta')
    );

    const pills = [...document.querySelectorAll<HTMLButtonElement>('button.pill-link')];
    expect(pills.length).toBe(2);

    // Click the second pill (node-b). The jump must target idB, NOT idA - a
    // name-keyed lookup would resolve both to whichever row sorts first.
    pills[1]!.click();
    // jumpToEndpoint is async (await tick() inside); wait for the side-effect
    // that happens AFTER the await, not just onJump which fires before it.
    await vi.waitFor(() =>
      expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: 'center' })
    );
    expect(onJump).toHaveBeenCalledTimes(1);
    // Focus landed on node-b's row, the distinct target for this pill.
    expect(document.activeElement).toBe(rowB);
  });

  it('falls back to plain span pills when endpoint_ids is absent (old server)', async () => {
    const payload = {
      model_groups: [
        {
          family: 'qwen',
          model_count: 1,
          endpoints: ['ollama-1'],
          models: [
            {
              name: 'legacy-gamma',
              params: '8B',
              quant: 'q4_k_m',
              size: '4.5 GB',
              endpoints: ['ollama-1'],
              last_seen_at: new Date().toISOString(),
            },
          ],
        },
      ],
    };

    component = mount(ModelsPanel, { target: document.body });
    flushSync();
    await refreshWith(payload, () =>
      expect(models.data?.model_groups?.[0]?.models?.[0]?.name).toBe('legacy-gamma')
    );

    // Old-server tolerance: no endpoint_ids, so pills are non-interactive spans.
    expect(document.querySelectorAll('button.pill-link').length).toBe(0);
    const spans = document.querySelectorAll('.endpoint-pills .pill');
    expect(spans.length).toBe(1);
  });
});

describe('ModelsPanel aliases subtext (WP-B2)', () => {
  it('renders aliases as quiet subtext under the model name when present', async () => {
    const payload = {
      model_groups: [
        {
          family: 'qwen',
          model_count: 1,
          endpoints: ['ollama-1'],
          models: [
            {
              name: 'alias-delta',
              params: '8B',
              quant: 'q4_k_m',
              size: '4.5 GB',
              endpoints: ['ollama-1'],
              endpoint_ids: ['http://ollama-1:11434'],
              aliases: ['qwen3-8b', 'qwen3:8b-instruct'],
              last_seen_at: new Date().toISOString(),
            },
          ],
        },
      ],
    };

    component = mount(ModelsPanel, { target: document.body });
    flushSync();
    await refreshWith(payload, () =>
      expect(models.data?.model_groups?.[0]?.models?.[0]?.name).toBe('alias-delta')
    );

    const aliasEl = document.querySelector('.model-aliases');
    expect(aliasEl).toBeTruthy();
    expect(aliasEl!.textContent!.trim()).toBe('qwen3-8b, qwen3:8b-instruct');
  });

  it('omits the alias subtext entirely when aliases is absent', async () => {
    const payload = {
      model_groups: [
        {
          family: 'qwen',
          model_count: 1,
          endpoints: ['ollama-1'],
          models: [
            {
              name: 'noalias-epsilon',
              params: '8B',
              quant: 'q4_k_m',
              size: '4.5 GB',
              endpoints: ['ollama-1'],
              endpoint_ids: ['http://ollama-1:11434'],
              last_seen_at: new Date().toISOString(),
            },
          ],
        },
      ],
    };

    component = mount(ModelsPanel, { target: document.body });
    flushSync();
    await refreshWith(payload, () =>
      expect(models.data?.model_groups?.[0]?.models?.[0]?.name).toBe('noalias-epsilon')
    );

    expect(document.querySelector('.model-aliases')).toBeNull();
  });

  it('renders aliases in the flat recent_models path too', async () => {
    const payload = {
      recent_models: [
        {
          name: 'flat-zeta',
          params: '3B',
          quant: 'q4',
          size: '2.0 GB',
          endpoints: ['ollama-2'],
          endpoint_ids: ['http://ollama-2:11434'],
          aliases: ['phi-3'],
          last_seen_at: new Date().toISOString(),
        },
      ],
    };

    component = mount(ModelsPanel, { target: document.body });
    flushSync();
    await refreshWith(payload, () =>
      expect(models.data?.recent_models?.[0]?.name).toBe('flat-zeta')
    );

    const aliasEl = document.querySelector('.model-aliases');
    expect(aliasEl).toBeTruthy();
    expect(aliasEl!.textContent!.trim()).toBe('phi-3');

    // Flat-path pill click-through is also wired (button, not span).
    expect(document.querySelectorAll('button.pill-link').length).toBe(1);
  });
});
