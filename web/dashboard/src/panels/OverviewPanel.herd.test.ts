import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';
import { overview } from '../lib/stores/overview.svelte';
import { endpoints } from '../lib/stores/endpoints.svelte';
import OverviewPanel from './OverviewPanel.svelte';
import { COLUMNS_STORAGE_KEY } from '../lib/herd';
import { endpointFilter } from '../lib/stores/endpoint-filter.svelte';

// U2: optional herd columns, column picker, issues tooltip and filter.

let component: ReturnType<typeof mount> | undefined;
beforeEach(() => localStorage.clear());
afterEach(() => {
  if (component) unmount(component);
  endpointFilter.clear();
  document.body.innerHTML = '';
});

const ok = (body: unknown) => ({ status: 200, ok: true, headers: { get: () => null }, json: async () => body });

const eps = [
  {
    name: 'gpu-box', type: 'ollama', status: 'healthy', health_check: 'ok', response_time: '12ms',
    success_rate: '98.0%', url: 'http://a', id: 'id-a', priority: 100, model_count: 3,
    request_count: 1000, min_latency_ms: 0, max_latency_ms: 0, active_connections: 2, avg_latency_ms: 40,
  },
  {
    name: 'mac-mini', type: 'vllm', status: 'offline', health_check: 'fail', success_rate: 'N/A',
    issues: 'unavailable', url: 'http://b', id: 'id-b', priority: 50, model_count: 0,
    request_count: 0, min_latency_ms: 0, max_latency_ms: 0, active_connections: 0,
  },
];

async function load() {
  global.fetch = vi.fn(async (url: RequestInfo | URL) => {
    if (String(url).includes('/internal/status/endpoints')) {
      return ok({ endpoints: eps, total_count: 2, healthy_count: 1, routable_count: 1 });
    }
    return ok({
      system: {
        status: 'healthy', endpoints_up: '1/2', success_rate: '98.0%', avg_latency: '40ms',
        total_traffic: '21.09 KB', active_connections: 2, total_requests: 1000, total_failures: 20,
        security_violations: 0, version: 'v', commit: 'c', start_time: new Date().toISOString(),
      },
      proxy: { engine: 'olla', balancer: 'priority' },
      endpoints: [
        { id: 'id-a', name: 'gpu-box', traffic: '21.09 KB' },
        { id: 'id-b', name: 'mac-mini', traffic: '0 B' },
      ],
    });
  });
  overview.refresh();
  endpoints.refresh();
  await vi.waitFor(() => {
    expect(overview.data?.system?.status).toBe('healthy');
    expect(endpoints.data?.endpoints?.length).toBe(2);
  });
  flushSync();
}

const headers = () => [...document.querySelectorAll('table.glance thead th')].map((th) => th.textContent?.replace(/[↕▲▼]/g, '').trim());

describe('herd columns', () => {
  it('shows the four core and four default optional columns', async () => {
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    expect(headers()).toEqual(['Endpoint', 'Status', 'Resp. rate', 'Avg latency', 'In flight', 'Requests', 'Type', 'Models']);
  });

  it('honours a stored selection and ignores stale keys', async () => {
    localStorage.setItem(COLUMNS_STORAGE_KEY, JSON.stringify(['traffic_bytes', 'bogus', 'failures']));
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    expect(headers().slice(4)).toEqual(['Traffic', 'Failures']);
    const row = document.querySelector('table.glance tbody tr')!;
    // Unhealthy sorts first: mac-mini has 0 B traffic and no failures figure.
    expect(row.textContent).toContain('0 B');
    expect(document.body.textContent).toContain('21.09 KB');
    // 1000 * (1 - 0.98), marked as an estimate; mac-mini has no traffic to estimate from.
    const cells = [...document.querySelectorAll('table.glance tbody tr')].map((r) => [...r.querySelectorAll('td')].at(-1)?.textContent?.trim());
    expect(cells).toEqual(['—', '~20']);
  });

  it('toggling the picker updates the table and persists', async () => {
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    document.querySelector<HTMLButtonElement>('.picker-btn')!.click();
    flushSync();
    const boxes = [...document.querySelectorAll<HTMLInputElement>('.picker-row input')];
    const priority = boxes.find((b) => b.parentElement?.textContent?.includes('Priority'))!;
    priority.click();
    flushSync();
    expect(headers()).toContain('Priority');
    expect(JSON.parse(localStorage.getItem(COLUMNS_STORAGE_KEY)!)).toContain('priority');
  });

  it('marks optional columns hide-narrow and not the core four', async () => {
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    const ths = [...document.querySelectorAll('table.glance thead th')];
    expect(ths.slice(0, 4).some((t) => t.classList.contains('hide-narrow'))).toBe(false);
    expect(ths.slice(4).every((t) => t.classList.contains('hide-narrow'))).toBe(true);
  });
});

describe('picker and sort', () => {
  it('labels the probe column in full and shows the failures estimate with a tilde', async () => {
    localStorage.setItem(COLUMNS_STORAGE_KEY, JSON.stringify(['probe_ms']));
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    expect(headers()).toContain('Probe latency');
  });

  it('closes the picker when focus moves outside it', async () => {
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    document.querySelector<HTMLButtonElement>('.picker-btn')!.click();
    flushSync();
    expect(document.querySelector('.picker-pop')).not.toBeNull();
    const box = document.querySelector<HTMLInputElement>('.picker-row input')!;
    const outside = document.querySelector<HTMLInputElement>('.filter-input')!;
    box.dispatchEvent(new FocusEvent('focusout', { bubbles: true, relatedTarget: outside }));
    flushSync();
    expect(document.querySelector('.picker-pop')).toBeNull();
  });

  it('closes the picker when focus leaves to nothing, but not during a press inside it', async () => {
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    const open = () => {
      document.querySelector<HTMLButtonElement>('.picker-btn')!.click();
      flushSync();
    };
    open();
    const box = document.querySelector<HTMLInputElement>('.picker-row input')!;
    box.dispatchEvent(new FocusEvent('focusout', { bubbles: true, relatedTarget: null }));
    flushSync();
    expect(document.querySelector('.picker-pop')).toBeNull();

    open();
    const pop = document.querySelector<HTMLElement>('.column-picker')!;
    pop.dispatchEvent(new Event('pointerdown', { bubbles: true }));
    document.querySelector('.picker-row input')!.dispatchEvent(new FocusEvent('focusout', { bubbles: true, relatedTarget: null }));
    flushSync();
    expect(document.querySelector('.picker-pop')).not.toBeNull();
  });

  it('toggles from the label text and the row, stays open, and keeps focus put on press', async () => {
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    document.querySelector<HTMLButtonElement>('.picker-btn')!.click();
    flushSync();
    const rows = () => [...document.querySelectorAll<HTMLElement>('.picker-row')];
    const priority = () => rows().find((r) => r.textContent?.includes('Priority'))!;

    // Real browsers move focus to the focusable panel on mousedown over label text.
    const text = priority().querySelector('span')!;
    const md = new MouseEvent('mousedown', { bubbles: true, cancelable: true });
    text.dispatchEvent(md);
    expect(md.defaultPrevented).toBe(true);
    const root = document.querySelector('.column-picker')!;
    root.dispatchEvent(new Event('pointerdown', { bubbles: true }));
    document.querySelector('.picker-btn')!.dispatchEvent(
      new FocusEvent('focusout', { bubbles: true, relatedTarget: document.getElementById('panel-overview') })
    );
    text.click();
    flushSync();
    window.dispatchEvent(new Event('pointerup'));
    expect(document.querySelector('.picker-pop')).not.toBeNull();
    expect(JSON.parse(localStorage.getItem(COLUMNS_STORAGE_KEY)!)).toContain('priority');

    // Clicking the row itself (padding) also toggles it back off.
    priority().click();
    flushSync();
    expect(document.querySelector('.picker-pop')).not.toBeNull();
    expect(JSON.parse(localStorage.getItem(COLUMNS_STORAGE_KEY)!)).not.toContain('priority');

    // With no press in flight, focus landing outside still closes it.
    document.querySelector('.picker-btn')!.dispatchEvent(
      new FocusEvent('focusout', { bubbles: true, relatedTarget: document.getElementById('panel-overview') })
    );
    flushSync();
    expect(document.querySelector('.picker-pop')).toBeNull();
  });

  it('Escape closes the picker and refocuses its button while focus is outside it', async () => {
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    const btn = document.querySelector<HTMLButtonElement>('.picker-btn')!;
    btn.click();
    flushSync();
    document.getElementById('panel-overview')!.focus();
    document.getElementById('panel-overview')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    flushSync();
    expect(document.querySelector('.picker-pop')).toBeNull();
    expect(document.activeElement).toBe(btn);
  });

  it('closes when focus lands directly on an element outside the picker', async () => {
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    document.querySelector<HTMLButtonElement>('.picker-btn')!.click();
    flushSync();
    document.querySelector<HTMLInputElement>('.filter-input')!.focus();
    flushSync();
    expect(document.querySelector('.picker-pop')).toBeNull();
  });

  it('does not cancel mousedown on the Reset button or a checkbox, only on label text', async () => {
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    document.querySelector<HTMLButtonElement>('.picker-btn')!.click();
    flushSync();
    const down = (el: Element) => {
      const e = new MouseEvent('mousedown', { bubbles: true, cancelable: true });
      el.dispatchEvent(e);
      return e.defaultPrevented;
    };
    expect(down(document.querySelector('.picker-reset')!)).toBe(false);
    expect(down(document.querySelector('.picker-row input')!)).toBe(false);
    expect(down(document.querySelector('.picker-row span')!)).toBe(true);
  });

  it('falls back to the default sort when the sorted column is hidden', async () => {
    localStorage.setItem(COLUMNS_STORAGE_KEY, JSON.stringify(['request_count']));
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    const sortBtn = [...document.querySelectorAll<HTMLButtonElement>('table.glance .sort-btn')].find((b) => b.textContent?.includes('Requests'))!;
    sortBtn.click();
    flushSync();
    expect(document.querySelector('th[aria-sort="descending"]')?.textContent).toContain('Requests');
    // Untick Requests via the picker.
    document.querySelector<HTMLButtonElement>('.picker-btn')!.click();
    flushSync();
    const box = [...document.querySelectorAll<HTMLInputElement>('.picker-row input')].find((b) => b.parentElement?.textContent?.includes('Requests'))!;
    box.click();
    flushSync();
    expect(document.querySelector('th[aria-sort="ascending"]')?.textContent).toContain('Status');
  });
});

describe('narrow viewport', () => {
  it('resets the sort when its column is hidden by the narrow layout', async () => {
    let listener: ((e: { matches: boolean }) => void) | undefined;
    window.matchMedia = vi.fn(() => ({
      matches: false,
      addEventListener: (_: string, cb: (e: { matches: boolean }) => void) => (listener = cb),
      removeEventListener: () => {},
    })) as unknown as typeof window.matchMedia;
    localStorage.setItem(COLUMNS_STORAGE_KEY, JSON.stringify(['request_count']));
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    [...document.querySelectorAll<HTMLButtonElement>('table.glance .sort-btn')].find((b) => b.textContent?.includes('Requests'))!.click();
    flushSync();
    expect(document.querySelector('th[aria-sort="descending"]')?.textContent).toContain('Requests');
    listener!({ matches: true });
    flushSync();
    expect(document.querySelector('th[aria-sort="ascending"]')?.textContent).toContain('Status');
  });
});

describe('issues tooltip', () => {
  it('is a focusable button described by a tooltip carrying the issue', async () => {
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    const btn = document.querySelector<HTMLButtonElement>('button.status-tag.has-issues')!;
    expect(btn).not.toBeNull();
    const tip = document.getElementById(btn.getAttribute('aria-describedby')!)!;
    expect(tip.getAttribute('role')).toBe('tooltip');
    expect(tip.textContent).toContain('unavailable');
    expect(tip.classList.contains('open')).toBe(false);
    btn.focus();
    btn.dispatchEvent(new FocusEvent('focus'));
    flushSync();
    expect(tip.classList.contains('open')).toBe(true);
    btn.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    flushSync();
    expect(tip.classList.contains('open')).toBe(false);
  });

  it('healthy rows get a plain pill with no button', async () => {
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    expect(document.querySelectorAll('button.status-tag.has-issues')).toHaveLength(1);
  });
});

describe('herd filter', () => {
  it('filters by text, by unhealthy chip, and shows an empty state', async () => {
    await load();
    component = mount(OverviewPanel, { target: document.body });
    flushSync();
    const rows = () => document.querySelectorAll('table.glance tbody tr').length;
    expect(rows()).toBe(2);

    const input = document.querySelector<HTMLInputElement>('.filter-input')!;
    input.value = 'gpu';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    expect(rows()).toBe(1);

    input.value = 'nope';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    expect(rows()).toBe(0);
    expect(document.querySelector('.herd-empty')).not.toBeNull();

    input.value = '';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    const unhealthy = [...document.querySelectorAll<HTMLButtonElement>('.chip')].find((c) => c.textContent === 'Unhealthy')!;
    unhealthy.click();
    flushSync();
    expect(rows()).toBe(1);
    expect(document.querySelector('table.glance tbody')!.textContent).toContain('mac-mini');
    expect(unhealthy.getAttribute('aria-pressed')).toBe('true');
  });
});
