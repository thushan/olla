import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach } from 'vitest';
import SparkStrip from './SparkStrip.svelte';
import type { Sample } from '../lib/stores/history.svelte';

// Samples are injected via the `samples` prop rather than the singleton store
// so the rendering assertions are deterministic and independent of poll
// timing. The production path (no prop) reads history.samples via the same
// $derived, so the prop only bypasses the store for tests.

let component: ReturnType<typeof mount> | undefined;

afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
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

  it('omits the readout entirely when no samples exist yet', () => {
    render({ samples: [] });
    expect(document.body.textContent).not.toContain('req/s');
  });
});
