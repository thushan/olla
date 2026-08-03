import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach } from 'vitest';
import RangeBar from './RangeBar.svelte';

// Regression coverage for finding 6: `avg` used to default to 0 and get fed
// straight into fmtMs(), so a genuinely-missing average (older backend, or
// an endpoint with no traffic yet) rendered as a confident "0ms" instead of
// a no-data placeholder - and the caller had derived that 0 by parseInt()-ing
// a health-check latency string in the first place (see EndpointsPanel).

interface RangeBarProps {
  min?: number;
  avg?: number | null;
  max?: number;
  globalMax?: number;
  label?: string;
}

let component: ReturnType<typeof mount> | undefined;

afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
});

function render(props: RangeBarProps): void {
  component = mount(RangeBar, { target: document.body, props });
  flushSync();
}

describe('RangeBar', () => {
  it('renders the numeric average, formatted, alongside the range', () => {
    render({ min: 10, avg: 288, max: 900, globalMax: 900 });
    expect(document.body.textContent).toContain('288ms');
    expect(document.body.textContent).toContain('900ms');
    const bar = document.querySelector('[role="img"]')!;
    expect(bar.getAttribute('aria-label')).toBe('latency: average 288ms, range 10ms to 900ms');
  });

  it('formats a >=1000ms average using the same seconds form as the API', () => {
    render({ min: 10, avg: 1500, max: 2000, globalMax: 2000 });
    expect(document.body.textContent).toContain('1.5s');
  });

  it('shows a placeholder, not "0ms", when avg_latency_ms is null (no traffic yet)', () => {
    render({ min: 0, avg: null, max: 500, globalMax: 500 });
    const avgText = document.querySelector('.range-labels')!.firstChild!.textContent!.trim();
    expect(avgText).toBe('—');
    const bar = document.querySelector('[role="img"]')!;
    expect(bar.getAttribute('aria-label')).toBe('latency: average —, range 0ms to 500ms');
  });

  it('shows a placeholder when avg_latency_ms is absent entirely (older backend)', () => {
    render({ min: 0, max: 500, globalMax: 500 });
    const avgText = document.querySelector('.range-labels')!.firstChild!.textContent!.trim();
    expect(avgText).toBe('—');
  });

  it('falls back to the full no-data state when there is no traffic at all', () => {
    render({ min: 0, avg: null, max: 0, globalMax: 0 });
    expect(document.body.textContent).toContain('no data');
    expect(document.querySelector('[role="img"]')).toBeNull();
  });
});
