import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, beforeAll, vi } from 'vitest';
import { models } from '../lib/stores/models.svelte';
import { navigation } from '../lib/stores/navigation.svelte';
import { stableId } from '../lib/dom-id';
import ModelsPanel from './ModelsPanel.svelte';

// Coverage: endpoint pills click-through to the matching EndpointsPanel
// row by STABLE ID (not display name). Two endpoints can share a display name
// (EndpointsPanel.dup-names.test), so a name->id lookup is forbidden; the
// backend pairs endpoints[i] with endpoint_ids[i], and each pill resolves to
// ep-${stableId(endpoint_id)}.
//
// Production double-hash: the server emits endpoint_ids as ALREADY-hashed
// opaque tokens (Go's stableEndpointID(url), e.g. "w5c2gc"), and EndpointsPanel
// renders each row's DOM id as ep-${stableId(endpoint_id)} - i.e. the client
// applies stableId to the server's opaque id. So endpoint_ids in these tests
// carry distinct opaque tokens (sid-aaa / sid-bbb, NOT raw urls), and planted
// target rows use ep-${stableId('sid-aaa')} etc. This mirrors the wire path
// pill -> stableId(endpoint_id) -> row, and would catch a regression in either
// hash. The same-name collision case below must land on DISTINCT rows via the
// two distinct ids - the exact failure mode a name-keyed lookup would silently
// get wrong.

// jsdom doesn't implement scrollIntoView; stub it so the jump helper doesn't
// throw and so the test can assert it ran.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

// The shared jump helper now consults endpoints.hasData before DOM-polling for
// the target row (cold deep-link wait). These tests pre-plant the row and
// exercise the fast path, so report hasData=true to bypass the cold wait.
vi.mock('../lib/stores/endpoints.svelte', () => ({
  endpoints: {
    get hasData() {
      return true;
    },
  },
}));

let component: ReturnType<typeof mount> | undefined;
afterEach(() => {
  if (component) unmount(component);
  component = undefined;
  document.body.innerHTML = '';
  navigation.set('overview');
  history.replaceState(null, '', '#overview');
  vi.useRealTimers();
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

describe('ModelsPanel endpoint pill click-through', () => {
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
              endpoint_ids: ['sid-aaa', 'sid-bbb'],
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
    // The ids mirror the production wire path: the server's opaque token is
    // hashed again by EndpointsPanel's rowDomId, so we assert against that
    // double-hashed DOM id, not against the raw token.
    const idA = `ep-${stableId('sid-aaa')}`;
    const idB = `ep-${stableId('sid-bbb')}`;
    expect(idA).not.toBe(idB);
  });

  it('navigates to endpoints and targets the clicked pill endpoint id', async () => {
    // Plant the two target rows in the DOM so jumpToEndpoint's getElementById
    // resolves, and capture which id was looked up. Rows use the production
    // double-hash: server opaque token -> client stableId -> DOM id.
    const idA = `ep-${stableId('sid-aaa')}`;
    const idB = `ep-${stableId('sid-bbb')}`;
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
              endpoint_ids: ['sid-aaa', 'sid-bbb'],
              last_seen_at: new Date().toISOString(),
            },
          ],
        },
      ],
    };

    component = mount(ModelsPanel, { target: document.body });
    flushSync();
    await refreshWith(payload, () =>
      expect(models.data?.model_groups?.[0]?.models?.[0]?.name).toBe('collide-beta')
    );

    const pills = [...document.querySelectorAll<HTMLButtonElement>('button.pill-link')];
    expect(pills.length).toBe(2);

    // Click the second pill (sid-bbb). The jump must target idB, NOT idA - a
    // name-keyed lookup would resolve both to whichever row sorts first.
    pills[1]!.click();
    // The shared helper is async (tick + retry); wait for the side-effect
    // that happens once the row resolves.
    await vi.waitFor(() =>
      expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: 'center' })
    );
    expect(navigation.current).toBe('endpoints');
    expect(location.hash).toBe(`#endpoints/${idB}`);
    // Focus landed on sid-bbb's row, the distinct target for this pill.
    expect(document.activeElement).toBe(rowB);
    // Pills carry the endpoint id in the DOM so the click target is
    // inspectable, not buried in the JS closure.
    expect(pills[1]!.getAttribute('data-endpoint-id')).toBe('sid-bbb');
  });

  it('retries the row lookup when the Endpoints panel fetch lands after the click', async () => {
    // First-navigation race: EndpointsPanel only starts its store on mount and
    // the fetch is async, so the target row is absent immediately after the
    // panel swap. The shared helper must retry until the row appears, then
    // focus it - otherwise the jump silently no-ops (panel swaps, no
    // scroll/focus). Fake timers make the retry deterministic.
    vi.useFakeTimers();
    const idB = `ep-${stableId('sid-bbb')}`;
    const rowB = document.createElement('div');
    rowB.id = idB;
    rowB.tabIndex = -1;

    const payload = {
      model_groups: [
        {
          family: 'qwen',
          model_count: 1,
          endpoints: ['ollama', 'ollama'],
          models: [
            {
              name: 'race-eta',
              params: '8B',
              quant: 'q4_k_m',
              size: '4.5 GB',
              endpoints: ['ollama', 'ollama'],
              endpoint_ids: ['sid-aaa', 'sid-bbb'],
              last_seen_at: new Date().toISOString(),
            },
          ],
        },
      ],
    };

    component = mount(ModelsPanel, { target: document.body });
    flushSync();
    await refreshWith(payload, () =>
      expect(models.data?.model_groups?.[0]?.models?.[0]?.name).toBe('race-eta')
    );

    const pills = [...document.querySelectorAll<HTMLButtonElement>('button.pill-link')];
    expect(pills.length).toBe(2);

    // Click BEFORE the target row exists - mirroring the first-nav race where
    // EndpointsPanel's fetch hasn't resolved yet.
    const p = Promise.resolve(pills[1]!.click());
    // Drain the tick + first retries (all miss because rowB is absent).
    await vi.advanceTimersByTimeAsync(110);
    expect(document.activeElement).not.toBe(rowB);
    // The fetch lands; the next retry finds rowB.
    document.body.append(rowB);
    await vi.advanceTimersByTimeAsync(60);
    await p;

    expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: 'center' });
    expect(document.activeElement).toBe(rowB);
    expect(navigation.current).toBe('endpoints');
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

describe('ModelsPanel aliases subtext', () => {
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
