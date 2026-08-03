import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import App from './App.svelte';
import { navigation } from './lib/stores/navigation.svelte.ts';

// Regression coverage for finding 11: App.svelte used to hold its own
// `current` state, separate from the `navigation` store NavTabs renders
// aria-selected/tabindex/arrow-key position from. Clicking an endpoint row
// on Overview (a programmatic jump) rendered the Endpoints panel while the
// tab bar kept announcing "Overview, selected" and held keyboard focus
// position there - it only self-healed on the next explicit tab click.

// App.svelte starts the poll scheduler and fetches on mount; stub fetch so
// that doesn't error out mid-test (the panels aren't what's under test here).
global.fetch = vi.fn(async () => ({
  status: 200,
  ok: true,
  headers: { get: () => null },
  json: async () => ({}),
}));

let component;
afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
  navigation.set('overview'); // leave the singleton store clean for other test files
});

describe('App navigation authority', () => {
  it('a programmatic jump updates both the rendered panel and the tab bar state', () => {
    component = mount(App, { target: document.body });
    flushSync();

    expect(document.getElementById('panel-overview')).not.toBeNull();
    expect(document.getElementById('tab-overview').getAttribute('aria-selected')).toBe('true');

    // Simulate a programmatic jump the way OverviewPanel's "jump to
    // endpoint" link does, via the same navigation authority App uses.
    navigation.set('endpoints');
    flushSync();

    // The panel actually rendered...
    expect(document.getElementById('panel-endpoints')).not.toBeNull();
    expect(document.getElementById('panel-overview')).toBeNull();
    // ...and the tab bar agrees, rather than still announcing Overview.
    expect(document.getElementById('tab-endpoints').getAttribute('aria-selected')).toBe('true');
    expect(document.getElementById('tab-overview').getAttribute('aria-selected')).toBe('false');
    expect(document.getElementById('tab-endpoints').getAttribute('tabindex')).toBe('0');
    expect(document.getElementById('tab-overview').getAttribute('tabindex')).toBe('-1');
  });

  it('arrow-key navigation continues from the correct tab after a programmatic jump', () => {
    component = mount(App, { target: document.body });
    flushSync();

    navigation.set('endpoints');
    flushSync();

    const tabs = document.querySelector('[role="tablist"]');
    tabs.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true }));
    flushSync();

    // From endpoints (index 1 of [overview, endpoints, models]), ArrowRight
    // must land on models, not restart from wherever App's stale `current`
    // used to think the tab bar was.
    expect(document.getElementById('panel-models')).not.toBeNull();
    expect(document.getElementById('tab-models').getAttribute('aria-selected')).toBe('true');
  });
});
