// Svelte 5 runes class. Module-scope singleton so every importer shares one
// piece of state. Lifted from the prior prototype's pattern, ported to a
// proper class with private state.

const STORAGE_KEY = 'olla-theme';
const MODES = ['auto', 'light', 'dark'];

function systemPrefersDark() {
  if (typeof window === 'undefined' || !window.matchMedia) return false;
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

function applyModeToDom(mode) {
  const root = document.documentElement;
  if (mode === 'auto') {
    root.removeAttribute('data-theme');
  } else {
    root.setAttribute('data-theme', mode);
  }
}

function readStored() {
  if (typeof localStorage === 'undefined') return 'auto';
  const v = localStorage.getItem(STORAGE_KEY);
  return MODES.includes(v) ? v : 'auto';
}

class ThemeStore {
  #mode = $state('auto');
  // Mirrors the OS preference as reactive state. `resolved` reading a plain
  // matchMedia() call (rather than this field) was the item-15 bug: Svelte
  // only reruns $derived/effects when a $state dependency changes, so a
  // getter that called window.matchMedia() directly never triggered a
  // recompute when the OS flipped mid-session, and the toggle's aria-label
  // went stale even though the CSS media query repainted the page correctly.
  #systemDark = $state(systemPrefersDark());
  #media = null;
  #onSystemChange = (e) => {
    this.#systemDark = e.matches;
  };

  constructor() {
    this.#mode = readStored();
    applyModeToDom(this.#mode);

    if (typeof window !== 'undefined' && window.matchMedia) {
      this.#media = window.matchMedia('(prefers-color-scheme: dark)');
      this.#media.addEventListener('change', this.#onSystemChange);
    }
  }

  get mode() {
    return this.#mode;
  }

  /** The effective theme after resolving "auto" against the OS preference. */
  get resolved() {
    return this.#mode === 'auto' ? (this.#systemDark ? 'dark' : 'light') : this.#mode;
  }

  /**
   * Flips to the opposite of whatever is currently on screen. Deliberately
   * not a 3-state auto->light->dark->auto cycle: when the system preference
   * already matched the next step in that cycle, the click was a no-op with
   * nothing visibly changing (e.g. system light, auto -> light). Toggling
   * against `resolved` instead guarantees every click changes what's
   * rendered, whether starting from auto or from an explicit override.
   */
  toggle() {
    this.set(this.resolved === 'dark' ? 'light' : 'dark');
  }

  /**
   * Wrapping 3-state cycle for the single-button theme control:
   * auto -> light -> dark -> auto. Unlike toggle(), the icon-only button's
   * glyph always reflects `mode` (not `resolved`), so each step is a visible
   * change even when the resolved theme happens to match the next mode.
   */
  cycle() {
    const order = ['auto', 'light', 'dark'];
    const next = order[(order.indexOf(this.#mode) + 1) % order.length];
    this.set(next);
  }

  set(mode) {
    if (!MODES.includes(mode)) return;
    this.#mode = mode;
    if (typeof localStorage !== 'undefined') {
      if (mode === 'auto') localStorage.removeItem(STORAGE_KEY);
      else localStorage.setItem(STORAGE_KEY, mode);
    }
    applyModeToDom(mode);
  }

  /** Detaches the matchMedia listener. Call on app teardown to avoid leaks. */
  destroy() {
    this.#media?.removeEventListener('change', this.#onSystemChange);
  }
}

export const theme = new ThemeStore();
