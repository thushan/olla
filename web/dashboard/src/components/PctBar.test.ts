import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach } from 'vitest';
import PctBar from './PctBar.svelte';

let component: ReturnType<typeof mount> | undefined;

afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
});

function render(props: { pct?: number; hasData?: boolean; status?: string }): void {
  component = mount(PctBar, { target: document.body, props });
  flushSync();
}

// The bar is display:none under 600px in the herd table, so the accessible
// name must sit on the cell that stays visible, and the bar must be hidden
// from assistive tech rather than carrying the label itself.
describe('PctBar accessible name', () => {
  it('labels the visible cell and hides the decorative bar', () => {
    render({ pct: 99.5, hasData: true, status: 'healthy' });
    const cell = document.querySelector('.pct-cell')!;
    expect(cell.getAttribute('role')).toBe('img');
    expect(cell.getAttribute('aria-label')).toBe('99.5% response rate');
    const bar = document.querySelector('.pct-bar')!;
    expect(bar.getAttribute('aria-hidden')).toBe('true');
    expect(bar.getAttribute('aria-label')).toBeNull();
  });

  it('labels the no-data dash as "no data"', () => {
    render({ hasData: false });
    const cell = document.querySelector('.pct-cell')!;
    expect(cell.getAttribute('aria-label')).toBe('no data');
    expect(cell.textContent?.trim()).toBe('—');
  });
});
