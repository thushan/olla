import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { models } from '../lib/stores/models.svelte';
import ModelsPanel from './ModelsPanel.svelte';

// Regression coverage: on a first-load fetch failure, models.data stays null,
// so groups/flatRecent both derive to [] and the panel fell through to the
// "No models discovered yet" empty-state copy - rendered right alongside
// StatusBanner's error banner. An operator reading both at once sees "the
// backends genuinely have no models" rather than "the request failed".

let component: ReturnType<typeof mount> | undefined;
afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
});

function emptyStateText() {
  return [...document.querySelectorAll<HTMLElement>('.panel-intro')]
    .map((el) => el.textContent!.trim())
    .join(' ');
}

describe('ModelsPanel failed-fetch state', () => {
  it('does not claim "no models discovered" when the fetch failed', async () => {
    global.fetch = vi.fn(async () => {
      throw new Error('network error');
    });

    component = mount(ModelsPanel, { target: document.body });
    flushSync();

    models.refresh();
    await vi.waitFor(() => expect(models.status).toBe('error'));
    flushSync();

    expect(models.hasData).toBe(false);
    expect(emptyStateText()).not.toContain('No models discovered yet');
    // The error banner must be the one and only story on screen.
    expect(document.querySelector('.banner.error')).toBeTruthy();
    expect(document.body.textContent).toContain("Couldn't reach");
  });
});
