import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  parseHash,
  serializeRoute,
  routeToHash,
  pushRoute,
  navigatePanel,
  startRouter,
  currentRoute,
} from './router';
import { navigation } from './stores/navigation.svelte';

describe('router parse/serialize', () => {
  it('round-trips every panel-only route', () => {
    for (const panel of ['overview', 'endpoints', 'models'] as const) {
      const r = { panel };
      expect(parseHash(routeToHash(r))).toEqual(r);
    }
  });

  it('round-trips an endpoint-targeted route', () => {
    const r = { panel: 'endpoints' as const, endpointKey: 'abc123' };
    expect(serializeRoute(r)).toBe('endpoints/ep-abc123');
    expect(parseHash('#endpoints/ep-abc123')).toEqual(r);
  });

  it('defaults to overview on empty or bare hash', () => {
    expect(parseHash('')).toEqual({ panel: 'overview' });
    expect(parseHash('#')).toEqual({ panel: 'overview' });
  });

  it('defaults to overview on an unrecognised panel', () => {
    expect(parseHash('#garbage')).toEqual({ panel: 'overview' });
    expect(parseHash('#settings')).toEqual({ panel: 'overview' });
  });

  it('drops the endpoint fragment when the panel is not endpoints', () => {
    expect(parseHash('#models/ep-abc')).toEqual({ panel: 'models' });
  });

  it('drops an endpoint key with invalid characters (defends the stableId shape)', () => {
    expect(parseHash('#endpoints/ep-Abc!')).toEqual({ panel: 'endpoints' });
    expect(parseHash('#endpoints/ep-with-dash')).toEqual({ panel: 'endpoints' });
  });

  it('keeps the ep- prefix out of the parsed key', () => {
    expect(parseHash('#endpoints/ep-w5c2gc').endpointKey).toBe('w5c2gc');
  });
});

describe('navigation <-> hash sync', () => {
  beforeEach(() => {
    history.replaceState(null, '', '#overview');
    navigation.set('overview');
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('navigatePanel sets the store and pushes a history entry', () => {
    navigatePanel('models');
    expect(navigation.current).toBe('models');
    expect(location.hash).toBe('#models');
  });

  it('pushRoute for an endpoint jump writes the full fragment', () => {
    pushRoute({ panel: 'endpoints', endpointKey: 'w5c2gc' });
    expect(location.hash).toBe('#endpoints/ep-w5c2gc');
  });

  it('hashchange updates the store on back/forward', () => {
    const stop = startRouter(() => {});
    history.replaceState(null, '', '#models');
    window.dispatchEvent(new Event('hashchange'));
    expect(navigation.current).toBe('models');
    stop();
  });

  it('hashchange with an endpoint route fires the restore callback', () => {
    const onEndpoint = vi.fn();
    const stop = startRouter(onEndpoint);
    history.replaceState(null, '', '#endpoints/ep-abc123');
    window.dispatchEvent(new Event('hashchange'));
    expect(navigation.current).toBe('endpoints');
    expect(onEndpoint).toHaveBeenCalledWith({
      panel: 'endpoints',
      endpointKey: 'abc123',
    });
    stop();
  });

  it('panel-only hashchange does not fire the endpoint callback', () => {
    const onEndpoint = vi.fn();
    const stop = startRouter(onEndpoint);
    history.replaceState(null, '', '#endpoints');
    window.dispatchEvent(new Event('hashchange'));
    expect(navigation.current).toBe('endpoints');
    expect(onEndpoint).not.toHaveBeenCalled();
    stop();
  });

  it('currentRoute reads the live hash', () => {
    history.replaceState(null, '', '#endpoints/ep-deadbeef');
    expect(currentRoute()).toEqual({
      panel: 'endpoints',
      endpointKey: 'deadbeef',
    });
  });
});
