// URL hash router for the dashboard SPA. The hash carries the active panel
// and, when the operator jumped to a specific endpoint, the target row's
// stableId - so a refresh or shared link restores the exact view, and the
// browser's back/forward traverses panel switches and endpoint jumps in
// arrival order.
//
// Scheme (round-trippable via parseHash/serializeRoute):
//   #overview                  panel only
//   #endpoints                 panel only
//   #models                    panel only
//   #endpoints/ep-<stableId>   panel + targeted endpoint row
//
// Sync model: user-initiated nav (tab click, jump-to-endpoint) calls
// pushRoute, which uses history.pushState. pushState is silent, so the
// hashchange listener only fires on genuine history traversal (back/forward)
// or a manual URL edit, never on a click-jump - that's what keeps a click
// from double-triggering the restore path.

import { navigation, type Section, SECTIONS } from './stores/navigation.svelte';

export interface Route {
  panel: Section;
  // The endpoint row's stableId (the post-hash DOM key), present only when
  // the route pins a specific endpoint row. Stored as the bare key so the
  // restore path can getElementById(`ep-${endpointKey}`) without re-hashing.
  endpointKey?: string;
}

const EP_PREFIX = 'ep-';

// stableId emits [0-9a-z]+; reuse that as the validity gate so a malformed
// fragment (percent-encoding, stray slashes, a copied URL truncated mid-key)
// degrades to a panel-only route instead of masquerading as a target.
const KEY_RE = /^[0-9a-z]+$/;

// Parse the location hash into a route. Tolerant: any unrecognised value
// falls back to overview so a stale or typo'd shared link never blanks the
// whole dashboard.
export function parseHash(hash: string): Route {
  const h = hash.replace(/^#/, '').trim();
  if (!h) return { panel: 'overview' };
  const slash = h.indexOf('/');
  const head = slash === -1 ? h : h.slice(0, slash);
  const tail = slash === -1 ? '' : h.slice(slash + 1);
  if (!SECTIONS.includes(head as Section)) return { panel: 'overview' };
  const panel = head as Section;
  if (panel === 'endpoints' && tail.startsWith(EP_PREFIX)) {
    const key = tail.slice(EP_PREFIX.length);
    if (KEY_RE.test(key)) return { panel, endpointKey: key };
  }
  return { panel };
}

// Serialise a route to the hash fragment (no leading '#').
export function serializeRoute(r: Route): string {
  if (r.panel === 'endpoints' && r.endpointKey) {
    return `endpoints/${EP_PREFIX}${r.endpointKey}`;
  }
  return r.panel;
}

export function routeToHash(r: Route): string {
  return `#${serializeRoute(r)}`;
}

export function currentRoute(): Route {
  return parseHash(typeof location !== 'undefined' ? location.hash : '');
}

// Push a new history entry for user-initiated navigation. pushState is silent
// (no hashchange/popstate), so the caller still owns the store update; this
// keeps click-jumps from re-entering the hashchange handler as if they were
// back/forward.
export function pushRoute(r: Route): void {
  if (typeof history === 'undefined') return;
  const url = routeToHash(r);
  if (url === location.hash) return;
  history.pushState(r, '', url);
}

// Programmatic panel switch (tab click, or a jump that targets a panel only).
// Updates the store and pushes a history entry so back/forward traverses the
// chain.
export function navigatePanel(panel: Section): void {
  navigation.set(panel);
  pushRoute({ panel });
}

// Install the hashchange listener for browser-driven navigation: back/forward
// across history entries, or a manual URL edit. On each event, re-sync the
// store from the hash and, when the route targets an endpoint, hand off to
// the caller's callback so it can scroll/focus/flash the row. Returns a
// teardown function.
export function startRouter(onEndpointRoute: (r: Route) => void): () => void {
  const handler = () => {
    const r = currentRoute();
    navigation.set(r.panel);
    if (r.endpointKey) onEndpointRoute(r);
  };
  window.addEventListener('hashchange', handler);
  return () => window.removeEventListener('hashchange', handler);
}
