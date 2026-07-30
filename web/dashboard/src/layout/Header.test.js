import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import Header from './Header.svelte';
import { startClock } from '../lib/clock.svelte.js';
import { pollScheduler } from '../lib/poll-scheduler.js';

// Regression coverage for finding 13: Header ran its own setInterval(...,
// 1000) instead of reading the shared clock, contradicting the stated
// invariant (App.svelte / clock.svelte.js) that the scheduler owns every
// timer in the SPA - and quietly skipping the visibility backoff every
// other live value gets for free.

let component;
afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
  vi.useRealTimers();
});

describe('Header clock', () => {
  it('creates no interval of its own, and updates when the shared clock ticks', () => {
    const setIntervalSpy = vi.spyOn(globalThis, 'setInterval');

    startClock(); // idempotent - App.svelte normally does this once at boot
    component = mount(Header, { target: document.body });
    flushSync();

    expect(setIntervalSpy).not.toHaveBeenCalled();

    const before = document.querySelector('.clock').textContent;

    vi.useFakeTimers();
    vi.setSystemTime(new Date('2030-06-15T10:20:30Z'));
    // Simulates the scheduler firing the clock job - this is the only
    // legitimate way `now` moves; Header must react to it via getNow(),
    // not its own timer.
    pollScheduler.refresh('clock');
    flushSync();

    const after = document.querySelector('.clock').textContent;
    expect(after).not.toBe(before);

    setIntervalSpy.mockRestore();
  });
});
