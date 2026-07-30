// Single scheduler for all poll stores. Spec §7.3 forbids one setInterval per
// store; this is the one-and-only timer owner (grep-provable: search for
// setInterval / setTimeout in src/ and they only appear here).
//
// Responsibilities:
//   - jitter each interval ±12% so multiple dashboard tabs don't synchronise
//   - back off to BACKGROUND_BACKOFF when the document is hidden
//   - fire an immediate refresh for every job when visibility returns
//   - abort an in-flight request before starting the next tick

const JITTER = 0.12;
const BACKGROUND_BACKOFF = 4; // multiplied against the configured interval
export const STALE_MULTIPLIER = 3; // last success older than 3*interval => stale

class PollScheduler {
  #jobs = new Map(); // name -> { intervalMs, tick }
  #timers = new Map(); // name -> timeout id (foreground cadence)
  #inflight = new Map(); // name -> AbortController
  #started = false;
  #boundOnVisibility = () => this.#onVisibility();

  register(name, intervalMs, tick) {
    this.#jobs.set(name, { intervalMs, tick });
  }

  start() {
    if (this.#started || typeof document === 'undefined') return;
    this.#started = true;
    document.addEventListener('visibilitychange', this.#boundOnVisibility);
    for (const name of this.#jobs.keys()) this.#scheduleNext(name, true);
  }

  stop() {
    if (!this.#started) return;
    this.#started = false;
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', this.#boundOnVisibility);
    }
    for (const [name, id] of this.#timers) clearTimeout(id);
    this.#timers.clear();
    for (const [, ctrl] of this.#inflight) ctrl.abort();
    this.#inflight.clear();
  }

  /** Force an immediate refresh of one job (used by manual "retry now" buttons). */
  refresh(name) {
    if (!this.#jobs.has(name)) return;
    // Cancel the pending recurring timer before running, mirroring
    // #onVisibility(). Without this the original timer still fires later,
    // and #run's own reschedule (see the guard there) adds a second live
    // chain - every "retry now" click leaked one more permanently.
    clearTimeout(this.#timers.get(name));
    this.#run(name);
  }

  #onVisibility() {
    if (document.visibilityState === 'visible') {
      // Tab is back: fire every job immediately and resume foreground cadence.
      for (const name of this.#jobs.keys()) {
        clearTimeout(this.#timers.get(name));
        this.#run(name);
      }
    }
  }

  #scheduleNext(name, immediate = false) {
    if (!this.#started) return;
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

  async #run(name) {
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
      // because a newer one - e.g. a manual refresh - started) must not also
      // reschedule: the newer run already owns that job, and both scheduling
      // is exactly how a single supersede used to spawn a second permanent
      // recurring chain.
      if (this.#inflight.get(name) === ctrl) {
        this.#inflight.delete(name);
        this.#scheduleNext(name);
      }
    }
  }
}

export const pollScheduler = new PollScheduler();
