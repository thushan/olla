import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

// Regression coverage for finding 7: refresh(name) (the "retry now" button)
// ran the job immediately without clearing the timer already queued by the
// previous #scheduleNext. The old timer still fired later, and since #run's
// finally unconditionally rescheduled, that produced a second live recurring
// chain - one extra permanent chain per manual retry, with no way to cancel
// the leaked ones.

beforeEach(() => {
  vi.resetModules();
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('pollScheduler.refresh', () => {
  it('a burst of manual refreshes leaves exactly one live recurring chain', async () => {
    const { pollScheduler } = await import('./poll-scheduler');

    let calls = 0;
    const tick = vi.fn(async () => {
      calls++;
    });
    pollScheduler.register('job', 1000, tick);
    pollScheduler.start();
    pollScheduler.start('job');

    // start() fires an immediate tick (delay 0).
    await vi.advanceTimersByTimeAsync(0);
    expect(calls).toBe(1);

    // Three manual "retry now" clicks in quick succession, each completing
    // before the next fires.
    pollScheduler.refresh('job');
    await vi.advanceTimersByTimeAsync(0);
    pollScheduler.refresh('job');
    await vi.advanceTimersByTimeAsync(0);
    pollScheduler.refresh('job');
    await vi.advanceTimersByTimeAsync(0);
    expect(calls).toBe(4);

    // Advance one full interval plus the maximum jitter window. With the bug,
    // each refresh leaked a duplicate chain, so this window would fire 4
    // more ticks (one per leaked chain); fixed, it fires exactly one.
    const before = calls;
    await vi.advanceTimersByTimeAsync(1200);
    expect(calls - before).toBe(1);

    pollScheduler.stop();
  });

  it('a superseded in-flight run does not also schedule a duplicate chain', async () => {
    const { pollScheduler } = await import('./poll-scheduler');

    const resolvers: Array<() => void> = [];
    const tick = vi.fn(
      (signal: AbortSignal) =>
        new Promise<void>((resolve) => {
          resolvers.push(resolve);
          // Mirrors the real tick(): it catches AbortError internally from
          // the fetch and just returns, it never rejects.
          signal.addEventListener('abort', () => resolve());
        })
    );
    pollScheduler.register('job', 1000, tick);
    pollScheduler.start();
    pollScheduler.start('job');

    // Initial run starts; its tick promise is left pending (in-flight).
    await vi.advanceTimersByTimeAsync(0);
    expect(tick).toHaveBeenCalledTimes(1);

    // Refresh while the first tick is still in-flight - this aborts and
    // supersedes it before its own finally ever runs.
    pollScheduler.refresh('job');
    await vi.advanceTimersByTimeAsync(0);

    // Resolve whatever is left pending (the new run's tick).
    resolvers.forEach((r) => r());
    await vi.advanceTimersByTimeAsync(0);

    // Exactly one recurring chain should be alive now.
    const before = tick.mock.calls.length;
    await vi.advanceTimersByTimeAsync(1200);
    expect(tick.mock.calls.length - before).toBe(1);

    pollScheduler.stop();
  });
});

// Per-job start/stop. A job only polls while activated via start(name);
// stop(name) deactivates it and aborts the in-flight tick. The global start()
// /stop() still gates the visibility listener, but no longer schedules every
// registered job - only the active subset.
describe('pollScheduler per-job start/stop', () => {
  it('start(name) activates only that job; the others never fire', async () => {
    const { pollScheduler } = await import('./poll-scheduler');

    const tickA = vi.fn(async () => {});
    const tickB = vi.fn(async () => {});
    pollScheduler.register('a', 1000, tickA);
    pollScheduler.register('b', 1000, tickB);

    pollScheduler.start();
    expect(pollScheduler.isActive('a')).toBe(false);
    expect(pollScheduler.isActive('b')).toBe(false);

    pollScheduler.start('a');
    expect(pollScheduler.isActive('a')).toBe(true);
    expect(pollScheduler.isActive('b')).toBe(false);

    await vi.advanceTimersByTimeAsync(0);
    expect(tickA).toHaveBeenCalledTimes(1);
    expect(tickB).toHaveBeenCalledTimes(0);

    // b is still silent after several intervals.
    await vi.advanceTimersByTimeAsync(3000);
    expect(tickB).toHaveBeenCalledTimes(0);
    expect(tickA.mock.calls.length).toBeGreaterThan(1);

    pollScheduler.stop();
  });

  it('stop(name) deactivates the job, aborts its in-flight tick and halts its chain', async () => {
    const { pollScheduler } = await import('./poll-scheduler');

    const resolvers: Array<() => void> = [];
    const tick = vi.fn(
      (signal: AbortSignal) =>
        new Promise<void>((resolve) => {
          resolvers.push(resolve);
          signal.addEventListener('abort', () => resolve());
        })
    );
    pollScheduler.register('job', 1000, tick);
    pollScheduler.start();
    pollScheduler.start('job');

    // Leave the first tick pending (in-flight), then stop the job.
    await vi.advanceTimersByTimeAsync(0);
    expect(tick).toHaveBeenCalledTimes(1);

    pollScheduler.stop('job');
    expect(pollScheduler.isActive('job')).toBe(false);

    // The aborted in-flight run must resolve (abort listener) but NOT
    // reschedule, and no further ticks may fire.
    resolvers.forEach((r) => r());
    const before = tick.mock.calls.length;
    await vi.advanceTimersByTimeAsync(5000);
    expect(tick.mock.calls.length).toBe(before);

    pollScheduler.stop();
  });

  it('the always-on clock job keeps ticking after a data job is stopped', async () => {
    const { pollScheduler } = await import('./poll-scheduler');

    const clockTicks = vi.fn(() => {});
    const dataTicks = vi.fn(async () => {});
    pollScheduler.register('clock', 1000, clockTicks);
    pollScheduler.register('data', 1000, dataTicks);

    pollScheduler.start();
    pollScheduler.start('clock');
    pollScheduler.start('data');

    await vi.advanceTimersByTimeAsync(0);
    // Both fire once on activation.
    expect(clockTicks).toHaveBeenCalledTimes(1);
    expect(dataTicks).toHaveBeenCalledTimes(1);

    // Stop the data job only; the clock must continue.
    pollScheduler.stop('data');
    expect(pollScheduler.isActive('clock')).toBe(true);
    expect(pollScheduler.isActive('data')).toBe(false);

    const clockBefore = clockTicks.mock.calls.length;
    const dataBefore = dataTicks.mock.calls.length;
    await vi.advanceTimersByTimeAsync(3000);

    expect(clockTicks.mock.calls.length).toBeGreaterThan(clockBefore);
    expect(dataTicks.mock.calls.length).toBe(dataBefore);

    pollScheduler.stop();
  });

  it('start(name) before global start() defers the first tick until start() runs', async () => {
    const { pollScheduler } = await import('./poll-scheduler');

    const tick = vi.fn(async () => {});
    pollScheduler.register('job', 1000, tick);

    // Activate before the scheduler is globally started: job is marked
    // active but does not fire yet.
    pollScheduler.start('job');
    expect(pollScheduler.isActive('job')).toBe(true);
    await vi.advanceTimersByTimeAsync(5000);
    expect(tick).toHaveBeenCalledTimes(0);

    // Global start kicks every active job immediately.
    pollScheduler.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(tick).toHaveBeenCalledTimes(1);

    pollScheduler.stop();
  });
});
