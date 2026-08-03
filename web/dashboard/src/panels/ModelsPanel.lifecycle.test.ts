import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { pollScheduler } from '../lib/poll-scheduler';
import ModelsPanel from './ModelsPanel.svelte';

// WP-B3 acceptance: the models store is activated with the scheduler ONLY
// while the Models panel is mounted. The heaviest payload
// (models?detailed=true&group=family) must stop firing the moment the
// operator switches to Overview or Endpoints.

global.fetch = vi.fn(async () => ({
  status: 200,
  ok: true,
  headers: { get: () => null },
  json: async () => ({}),
}));

let component: ReturnType<typeof mount> | undefined;
afterEach(() => {
  if (component) unmount(component);
  component = undefined;
  document.body.innerHTML = '';
});

describe('ModelsPanel polling lifecycle (WP-B3)', () => {
  it('activates the models job on mount and deactivates on unmount', () => {
    expect(pollScheduler.isActive('models')).toBe(false);

    component = mount(ModelsPanel, { target: document.body });
    flushSync();

    expect(pollScheduler.isActive('models')).toBe(true);

    unmount(component);
    component = undefined;
    flushSync();

    expect(pollScheduler.isActive('models')).toBe(false);
  });

  it('does not activate the endpoints job (disjoint store)', () => {
    component = mount(ModelsPanel, { target: document.body });
    flushSync();

    expect(pollScheduler.isActive('models')).toBe(true);
    // Mounting Models must not incidentally start the endpoints poll.
    expect(pollScheduler.isActive('endpoints')).toBe(false);
  });
});
