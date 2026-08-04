import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import SparkStrip, { nearestSampleIndex } from './SparkStrip.svelte';
import { history } from '../lib/stores/history.svelte';
import type { Sample } from '../lib/stores/history.svelte';

// Samples are injected via the `samples` prop rather than the singleton store
// so the rendering assertions are deterministic and independent of poll
// timing. The production path (no prop) reads history.samples via the same
// $derived, so the prop only bypasses the store for tests.

let component: ReturnType<typeof mount> | undefined;

afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
  // history is a module-scoped singleton shared across every test in this
  // file. Without reset, samples appended by a mount that bypassed the prop
  // (or by the $effect firing on an unstubbed overview tick) leak into the
  // next test and corrupt its view-derived assertions.
  history.reset();
});

function render(props?: { samples?: Sample[] }): void {
  component = mount(SparkStrip, { target: document.body, props: props ?? {} });
  flushSync();
}

function makeSample(over: Partial<Sample> = {}): Sample {
  return {
    t: Date.now(),
    totalRequests: 0,
    totalBytes: 0,
    activeConnections: 0,
    reqPerSec: 0,
    bytesPerSec: 0,
    ...over,
  };
}

// jsdom returns all-zero rects by default; the pointer handler maps x via
// getBoundingClientRect, so tests need a deterministic width. This stubs the
// chart element's rect to a 100x48 box at the origin: clientX == fraction.
function stubChartRect(width = 100): HTMLElement {
  const chart = document.querySelector('.spark-chart') as HTMLElement;
  vi.spyOn(chart, 'getBoundingClientRect').mockReturnValue({
    left: 0,
    top: 0,
    width,
    height: 48,
    right: width,
    bottom: 48,
    x: 0,
    y: 0,
    toJSON: () => {},
  } as DOMRect);
  return chart;
}

describe('nearestSampleIndex (pure helper)', () => {
  it('maps fractions to the nearest sample index', () => {
    expect(nearestSampleIndex(0, 4)).toBe(0);
    expect(nearestSampleIndex(0.34, 4)).toBe(1); // 0.34 * 3 = 1.02 -> round 1
    expect(nearestSampleIndex(0.5, 4)).toBe(2); // 0.5 * 3 = 1.5 -> round 2
    expect(nearestSampleIndex(1, 4)).toBe(3);
  });

  it('clamps out-of-range fractions into [0, n-1]', () => {
    expect(nearestSampleIndex(-0.5, 4)).toBe(0);
    expect(nearestSampleIndex(1.5, 4)).toBe(3);
  });

  it('returns -1 for empty arrays, 0 for single-element arrays', () => {
    expect(nearestSampleIndex(0.5, 0)).toBe(-1);
    expect(nearestSampleIndex(0.5, 1)).toBe(0);
  });
});

describe('SparkStrip', () => {
  it('renders the SVG element immediately so the strip height is stable from first paint', () => {
    render({ samples: [] });
    const svg = document.querySelector('.spark-svg') as SVGSVGElement | null;
    expect(svg).toBeTruthy();
  });

  it('shows the gathering-data hint with fewer than 2 samples', () => {
    render({ samples: [makeSample({ reqPerSec: 1.5, activeConnections: 3 })] });
    expect(document.querySelector('.spark-hint')).toBeTruthy();
    expect(document.body.textContent).toContain('gathering data');
  });

  it('hides the hint and draws both paths once 2+ samples arrive', () => {
    render({
      samples: [
        makeSample({ reqPerSec: 1, activeConnections: 2 }),
        makeSample({ reqPerSec: 2, activeConnections: 3 }),
      ],
    });
    expect(document.querySelector('.spark-hint')).toBeNull();
    const area = document.querySelector('.spark-area') as SVGPathElement | null;
    const line = document.querySelector('.spark-line') as SVGPathElement | null;
    expect(area).toBeTruthy();
    expect(line).toBeTruthy();
    expect(area!.getAttribute('d')).toMatch(/^M 0 /);
    expect(line!.getAttribute('d')).toMatch(/^M 0 /);
  });

  it('reflects the latest sample in the aria-label and the readout', () => {
    render({
      samples: [
        makeSample({ reqPerSec: 1, activeConnections: 2 }),
        makeSample({ reqPerSec: 2.4, activeConnections: 3 }),
      ],
    });
    const chart = document.querySelector('.spark-chart') as HTMLElement;
    expect(chart.getAttribute('aria-label')).toContain('2.4 requests per second');
    expect(chart.getAttribute('aria-label')).toContain('3 active connections');
    expect(document.body.textContent).toContain('2.4 req/s');
    expect(document.body.textContent).toContain('3 conns');
  });

  it('renders a placeholder readout (not omitted) when no samples exist yet, to avoid header layout shift', () => {
    render({ samples: [] });
    const readout = document.querySelector('.spark-readout');
    expect(readout).toBeTruthy();
    expect(readout!.textContent).not.toContain('req/s');
    expect(readout!.textContent?.trim()).toBe('—');
  });

  it('renders the req/s y-axis ceiling label and a 0 baseline', () => {
    render({
      samples: [
        makeSample({ reqPerSec: 1, activeConnections: 2 }),
        makeSample({ reqPerSec: 5.0, activeConnections: 3 }),
        makeSample({ reqPerSec: 2, activeConnections: 1 }),
      ],
    });
    const topLabel = document.querySelector('.spark-axis-top') as HTMLElement;
    const bottomLabel = document.querySelector('.spark-axis-bottom') as HTMLElement;
    expect(topLabel.textContent).toBe('5.0');
    expect(bottomLabel.textContent).toBe('0');
  });

  it('rounds the ceiling label to an integer when maxReq >= 10', () => {
    render({
      samples: [makeSample({ reqPerSec: 5 }), makeSample({ reqPerSec: 42 })],
    });
    expect((document.querySelector('.spark-axis-top') as HTMLElement).textContent).toBe('42');
  });

  it('shows the hovered sample on pointermove and clears on pointerleave', () => {
    const now = Date.now();
    render({
      samples: [
        makeSample({ reqPerSec: 1, activeConnections: 2, t: now - 30000 }),
        makeSample({ reqPerSec: 5, activeConnections: 3, t: now - 20000 }),
        makeSample({ reqPerSec: 2, activeConnections: 7, t: now - 10000 }),
      ],
    });
    const chart = stubChartRect(100);

    // x=50 -> fraction 0.5 -> nearestSampleIndex(0.5, 3) = round(1) = 1, the
    // middle sample (5 req/s, 3 conns). bubbles:true is needed because Svelte 5
    // delegates bubbling events to a root listener.
    chart.dispatchEvent(new MouseEvent('pointermove', { clientX: 50, bubbles: true }));
    flushSync();

    const tooltip = document.querySelector('.spark-tooltip') as HTMLElement | null;
    expect(tooltip).toBeTruthy();
    expect(tooltip!.textContent).toContain('5.0 req/s');
    expect(tooltip!.textContent).toContain('3 conns');
    expect(tooltip!.textContent).toContain('ago');
    expect(document.querySelector('.spark-guide')).toBeTruthy();

    chart.dispatchEvent(new MouseEvent('pointerleave'));
    flushSync();
    expect(document.querySelector('.spark-tooltip')).toBeNull();
    expect(document.querySelector('.spark-guide')).toBeNull();
  });

  it('maps the pointer to the first sample at the far-left edge', () => {
    render({
      samples: [
        makeSample({ reqPerSec: 9, activeConnections: 1 }),
        makeSample({ reqPerSec: 5, activeConnections: 2 }),
        makeSample({ reqPerSec: 1, activeConnections: 3 }),
      ],
    });
    const chart = stubChartRect(100);
    chart.dispatchEvent(new MouseEvent('pointermove', { clientX: 0, bubbles: true }));
    flushSync();
    const tooltip = document.querySelector('.spark-tooltip') as HTMLElement | null;
    expect(tooltip).toBeTruthy();
    expect(tooltip!.textContent).toContain('9.0 req/s');
    expect(tooltip!.textContent).toContain('1 conns');
  });

  it('does not show a hover tooltip during warm-up (< 2 samples)', () => {
    render({ samples: [makeSample({ reqPerSec: 1, activeConnections: 2 })] });
    const chart = stubChartRect(100);
    chart.dispatchEvent(new MouseEvent('pointermove', { clientX: 50, bubbles: true }));
    flushSync();
    expect(document.querySelector('.spark-tooltip')).toBeNull();
  });
});

describe('SparkStrip outage markers', () => {
  it('draws a muted-red outage band across an error sample and breaks the area path', () => {
    render({
      samples: [
        makeSample({ reqPerSec: 3, activeConnections: 2 }),
        makeSample({ reqPerSec: 5, activeConnections: 3 }),
        makeSample({ reqPerSec: 0, activeConnections: 0, error: true }),
        makeSample({ reqPerSec: 4, activeConnections: 1 }),
        makeSample({ reqPerSec: 2, activeConnections: 2 }),
      ],
    });
    // One outage band covering the lone error at index 2.
    const bands = document.querySelectorAll('.spark-outage');
    expect(bands.length).toBe(1);
    // step = 100/(5-1) = 25; index 2 sits at x=50, band spans a half-step
    // either side -> x=37.5, width=25.
    const band = bands[0] as SVGRectElement;
    expect(parseFloat(band.getAttribute('x')!)).toBeCloseTo(37.5);
    expect(parseFloat(band.getAttribute('width')!)).toBeCloseTo(25);
    // A single-sample outage draws one down-edge tick at x=50; the recovery
    // edge is suppressed because drop and recovery are the same poll.
    const edges = document.querySelectorAll('.spark-outage-edge');
    expect(edges.length).toBe(1);
    expect((edges[0] as SVGLineElement).getAttribute('x1')).toBe('50');

    // The area path must contain two M commands (two runs of non-error
    // samples: indices 0-1 and 3-4), confirming the break.
    const area = document.querySelector('.spark-area') as SVGPathElement;
    const mCount = (area.getAttribute('d') ?? '').match(/M/g)?.length ?? 0;
    expect(mCount).toBe(2);

    // The edge line and connections line also break.
    const edge = document.querySelector('.spark-edge') as SVGPathElement;
    expect((edge.getAttribute('d') ?? '').match(/M/g)?.length ?? 0).toBe(2);
    const connLine = document.querySelector('.spark-line') as SVGPathElement;
    expect((connLine.getAttribute('d') ?? '').match(/M/g)?.length ?? 0).toBe(2);
  });

  it('coalesces consecutive error samples into a single band with drop and recovery edges', () => {
    render({
      samples: [
        makeSample({ reqPerSec: 3, activeConnections: 2 }),
        makeSample({ reqPerSec: 0, activeConnections: 0, error: true }),
        makeSample({ reqPerSec: 0, activeConnections: 0, error: true }),
        makeSample({ reqPerSec: 0, activeConnections: 0, error: true }),
        makeSample({ reqPerSec: 4, activeConnections: 1 }),
      ],
    });
    // Three adjacent errors -> one band, not three.
    expect(document.querySelectorAll('.spark-outage').length).toBe(1);
    // Drop edge at index 1 (x=25), recovery edge at index 3 (x=75).
    const edges = document.querySelectorAll('.spark-outage-edge');
    expect(edges.length).toBe(2);
    expect((edges[0] as SVGLineElement).getAttribute('x1')).toBe('25');
    expect((edges[1] as SVGLineElement).getAttribute('x1')).toBe('75');
  });

  it('does not draw an outage band when there are no error samples', () => {
    render({
      samples: [
        makeSample({ reqPerSec: 3, activeConnections: 2 }),
        makeSample({ reqPerSec: 5, activeConnections: 3 }),
      ],
    });
    expect(document.querySelectorAll('.spark-outage').length).toBe(0);
    expect(document.querySelectorAll('.spark-outage-edge').length).toBe(0);
    // Single continuous run: one M in the area path.
    const area = document.querySelector('.spark-area') as SVGPathElement;
    expect((area.getAttribute('d') ?? '').match(/M/g)?.length ?? 0).toBe(1);
  });

  it('shows "connection lost" in the tooltip when hovering an error sample', () => {
    render({
      samples: [
        makeSample({ reqPerSec: 3, activeConnections: 2 }),
        makeSample({ reqPerSec: 0, activeConnections: 0, error: true, t: Date.now() - 10000 }),
        makeSample({ reqPerSec: 4, activeConnections: 1 }),
      ],
    });
    const chart = stubChartRect(100);
    // x=50 -> fraction 0.5 -> nearestSampleIndex(0.5, 3) = round(1) = 1, the
    // error sample.
    chart.dispatchEvent(new MouseEvent('pointermove', { clientX: 50, bubbles: true }));
    flushSync();

    const tooltip = document.querySelector('.spark-tooltip') as HTMLElement | null;
    expect(tooltip).toBeTruthy();
    expect(tooltip!.textContent).toContain('connection lost');
    expect(tooltip!.textContent).not.toContain('req/s');
    // Guide line picks up the error colour.
    expect(document.querySelector('.spark-guide')?.classList.contains('is-error')).toBe(true);
  });

  it('resumes normal values after recovery (hover a post-error sample)', () => {
    render({
      samples: [
        makeSample({ reqPerSec: 3, activeConnections: 2 }),
        makeSample({ reqPerSec: 0, activeConnections: 0, error: true }),
        makeSample({ reqPerSec: 4.2, activeConnections: 5 }),
      ],
    });
    const chart = stubChartRect(100);
    // x=100 -> fraction 1 -> index 2, the post-recovery sample.
    chart.dispatchEvent(new MouseEvent('pointermove', { clientX: 100, bubbles: true }));
    flushSync();

    const tooltip = document.querySelector('.spark-tooltip') as HTMLElement | null;
    expect(tooltip).toBeTruthy();
    expect(tooltip!.textContent).toContain('4.2 req/s');
    expect(tooltip!.textContent).toContain('5 conns');
    expect(document.querySelector('.spark-guide')?.classList.contains('is-error')).toBe(false);
  });

  it('aria-label says "connection lost" when the latest sample is an error', () => {
    render({
      samples: [
        makeSample({ reqPerSec: 3, activeConnections: 2 }),
        makeSample({ reqPerSec: 0, activeConnections: 0, error: true }),
      ],
    });
    const chart = document.querySelector('.spark-chart') as HTMLElement;
    expect(chart.getAttribute('aria-label')).toBe('connection lost');
  });

  it('readout shows "connection lost" when the latest sample is an error', () => {
    render({
      samples: [
        makeSample({ reqPerSec: 3, activeConnections: 2 }),
        makeSample({ reqPerSec: 0, activeConnections: 0, error: true }),
      ],
    });
    expect(document.querySelector('.spark-readout')?.textContent).toContain('connection lost');
  });
});
