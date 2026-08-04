// Single scheduler for all poll stores, rather than one setInterval per
// store; this is the one-and-only timer owner (grep-provable: search for
// setInterval / setTimeout in src/ and they only appear here).
//
// Responsibilities:
//   - jitter each interval ±12% so multiple dashboard tabs don't synchronise
//   - back off to BACKGROUND_BACKOFF when the document is hidden
//   - fire an immediate refresh for every job when visibility returns
//   - abort an in-flight request before starting the next tick
//   - per-job start/stop so an inactive panel's store stops polling.
//     The global start()/stop() still gates the visibility listener and the
//     whole machinery; individual jobs are only scheduled while both the
//     scheduler is globally started AND the job is activated via start(name).

const JITTER = 0.12;
const BACKGROUND_BACKOFF = 4; // multiplied against the configured interval
export const STALE_MULTIPLIER = 3; // last success older than 3*interval => stale

type TickFn = (signal: AbortSignal) => void | Promise<void>;

interface Job {
  intervalMs: number;
  tick: TickFn;
}

class PollScheduler {
  #jobs = new Map<string, Job>(); // name -> { intervalMs, tick }
  #timers = new Map<string, ReturnType<typeof setTimeout>>(); // name -> timeout id (foreground cadence)
  #inflight = new Map<string, AbortController>(); // name -> AbortController
  // Per-job active set. A job only ticks while it's in here, regardless of
  // the global #started flag - this is what lets an inactive panel's store
  // stop polling without tearing down the whole scheduler. The always-on
  // clock (lib/clock.svelte.ts) and the overview store stay in here for the
  // app lifetime; endpoints/models are added/removed by their panels.
  #active = new Set<string>();
  #started = false;
  #boundOnVisibility = (): void => this.#onVisibility();

  register(name: string, intervalMs: number, tick: TickFn): void {
    this.#jobs.set(name, { intervalMs, tick });
  }

  /** With no argument: globally start the scheduler - attach the visibility
   *  listener and kick every currently-active job. With a name: activate just
   *  that job and fire it immediately. Per-job activation is what lets
   *  inactive panels stop polling without tearing down the scheduler. */
  start(name?: string): void {
    if (name !== undefined) {
      this.#active.add(name);
      if (this.#started) {
        clearTimeout(this.#timers.get(name));
        this.#scheduleNext(name, true);
      }
      return;
    }
    if (this.#started || typeof document === 'undefined') return;
    this.#started = true;
    document.addEventListener('visibilitychange', this.#boundOnVisibility);
    for (const n of this.#active) this.#scheduleNext(n, true);
  }

  /** With no argument: globally stop the scheduler - detach the listener and
   *  clear every timer/in-flight request. With a name: deactivate just that
   *  job, clear its timer and abort its in-flight tick; it stays registered. */
  stop(name?: string): void {
    if (name !== undefined) {
      this.#active.delete(name);
      clearTimeout(this.#timers.get(name));
      this.#timers.delete(name);
      const ctrl = this.#inflight.get(name);
      if (ctrl) ctrl.abort();
      this.#inflight.delete(name);
      return;
    }
    if (!this.#started) return;
    this.#started = false;
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', this.#boundOnVisibility);
    }
    for (const [, id] of this.#timers) clearTimeout(id);
    this.#timers.clear();
    for (const [, ctrl] of this.#inflight) ctrl.abort();
    this.#inflight.clear();
  }

  isActive(name: string): boolean {
    return this.#active.has(name);
  }

  /** Force an immediate refresh of one job (used by manual "retry now" buttons).
   *  Works regardless of active state so a one-off manual fetch isn't gated on
   *  the panel being mounted. */
  refresh(name: string): void {
    if (!this.#jobs.has(name)) return;
    // Cancel the pending recurring timer before running, mirroring
    // #onVisibility(). Without this the original timer still fires later,
    // and #run's own reschedule (see the guard there) adds a second live
    // chain - every "retry now" click leaked one more permanently.
    clearTimeout(this.#timers.get(name));
    this.#run(name);
  }

  #onVisibility(): void {
    if (document.visibilityState === 'visible') {
      // Tab is back: fire every ACTIVE job immediately and resume foreground cadence.
      for (const name of this.#active) {
        clearTimeout(this.#timers.get(name));
        this.#run(name);
      }
    }
  }

  #scheduleNext(name: string, immediate: boolean = false): void {
    if (!this.#started || !this.#active.has(name)) return;
    const job = this.#jobs.get(name);
    if (!job) return;

    const hidden = typeof document !== 'undefined' && document.visibilityState === 'hidden';
    const base = hidden ? job.intervalMs * BACKGROUND_BACKOFF : job.intervalMs;
    // Jitter is +/- so the long-run average matches the configured interval.
    const jitter = (Math.random() * 2 - 1) * JITTER * base;
    const delay = immediate ? 0 : Math.max(500, base + jitter);

    const id = setTimeout(() => {
      this.#run(name);
    }, delay);
    this.#timers.set(name, id);
  }

  async #run(name: string): Promise<void> {
    const job = this.#jobs.get(name);
    if (!job) return;

    // Cancel any in-flight request so a fresh tick supersedes a stale one.
    const previous = this.#inflight.get(name);
    if (previous) previous.abort();
    const ctrl = new AbortController();
    this.#inflight.set(name, ctrl);

    try {
      await job.tick(ctrl.signal);
    } finally {
      // Only the run that still owns the current controller may clear
      // inflight and schedule the next tick. A superseded run (aborted
      // because a newer one - e.g. a manual refresh, or a stop(name) -
      // started) must not also reschedule: the newer run already owns that
      // job, and both scheduling is exactly how a single supersede used to
      // spawn a second permanent recurring chain.
      if (this.#inflight.get(name) === ctrl) {
        this.#inflight.delete(name);
        // Re-arm only if the job is still active. A stop(name) that aborted
        // us also removed it from #active, so the chain ends here.
        if (this.#active.has(name)) this.#scheduleNext(name);
      }
    }
  }
}

export const pollScheduler: PollScheduler = new PollScheduler();
