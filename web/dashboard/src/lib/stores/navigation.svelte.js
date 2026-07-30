// Single source of truth for which panel is active - both the tab bar
// (NavTabs.svelte) and the router (App.svelte) read and write this store
// directly, so the rendered panel and the tab bar's aria-selected/tabindex
// can never drift apart (finding 11: App used to hold its own separate
// `current` for programmatic jumps like "jump to endpoints from Overview",
// so the tab bar kept announcing the old section over the new panel).
// Panels themselves must still never import this (spec §7.2.1): they take a
// plain callback prop instead (see OverviewPanel's onJumpToEndpoints), so the
// nav presentation can be swapped to a left sidebar without touching them.

export const SECTIONS = Object.freeze(['overview', 'endpoints', 'models']);

class NavigationStore {
  #current = $state('overview');

  get current() {
    return this.#current;
  }

  get sections() {
    return SECTIONS;
  }

  set(section) {
    if (!SECTIONS.includes(section)) return;
    this.#current = section;
  }
}

export const navigation = new NavigationStore();
