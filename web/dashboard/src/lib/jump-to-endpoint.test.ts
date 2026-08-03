import { describe, it, expect, beforeAll, beforeEach, afterEach, vi } from 'vitest';
import { jumpToEndpoint, jumpToEndpointKey, flashRow } from './jump-to-endpoint';
import { stableId } from './dom-id';
import { navigation } from './stores/navigation.svelte';

// jsdom doesn't implement scrollIntoView; stub it so the helper doesn't throw
// and so the tests can assert it was actually called.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

beforeEach(() => {
  document.body.innerHTML = '';
  navigation.set('overview');
  history.replaceState(null, '', '#overview');
  // scrollIntoView is stubbed once in beforeAll; clear the call log so each
  // test's "was called / was not called" assertion sees only its own jumps.
  vi.mocked(Element.prototype.scrollIntoView).mockClear();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('flashRow', () => {
  it('adds the flash class, then removes it once the hold elapses', () => {
    vi.useFakeTimers();
    const row = document.createElement('tr');
    document.body.append(row);

    flashRow(row);
    expect(row.classList.contains('ep-flash')).toBe(true);

    vi.advanceTimersByTime(2000);
    expect(row.classList.contains('ep-flash')).toBe(false);
  });

  it('clears an existing flash on another row before lighting the new one', () => {
    vi.useFakeTimers();
    const a = document.createElement('tr');
    const b = document.createElement('tr');
    document.body.append(a, b);

    flashRow(a);
    expect(a.classList.contains('ep-flash')).toBe(true);

    flashRow(b);
    expect(a.classList.contains('ep-flash')).toBe(false);
    expect(b.classList.contains('ep-flash')).toBe(true);
  });

  it('re-flashes the same row on a second call mid-fade', () => {
    vi.useFakeTimers();
    const row = document.createElement('tr');
    document.body.append(row);

    flashRow(row);
    vi.advanceTimersByTime(1000);
    expect(row.classList.contains('ep-flash')).toBe(true);

    flashRow(row);
    expect(row.classList.contains('ep-flash')).toBe(true);
    // The hold timer was reset, so the class is still present at what would
    // have been the original removal point.
    vi.advanceTimersByTime(1500);
    expect(row.classList.contains('ep-flash')).toBe(true);
    vi.advanceTimersByTime(500);
    expect(row.classList.contains('ep-flash')).toBe(false);
  });
});

describe('jumpToEndpoint', () => {
  it('swaps to endpoints, pushes the hash, scrolls, focuses and flashes a present row', async () => {
    const rawId = 'http://ollama-1:11434';
    const domId = `ep-${stableId(rawId)}`;
    const row = document.createElement('tr');
    row.id = domId;
    row.tabIndex = -1;
    document.body.append(row);

    await jumpToEndpoint(rawId);

    expect(navigation.current).toBe('endpoints');
    expect(location.hash).toBe(`#endpoints/${domId}`);
    expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: 'center' });
    expect(document.activeElement).toBe(row);
    expect(row.classList.contains('ep-flash')).toBe(true);
  });

  it('retries until the row appears, then scrolls/focuses/flashes', async () => {
    vi.useFakeTimers();
    const rawId = 'race-id';
    const domId = `ep-${stableId(rawId)}`;
    const row = document.createElement('tr');
    row.id = domId;
    row.tabIndex = -1;

    // Kick off the jump before the row exists - mirroring the first-nav race
    // where EndpointsPanel's fetch hasn't resolved yet.
    const p = jumpToEndpoint(rawId);
    // Drain the tick + the first couple of 50ms retries (all miss).
    await vi.advanceTimersByTimeAsync(110);
    expect(document.activeElement).not.toBe(row);

    // The fetch lands; the next retry tick finds the row.
    document.body.append(row);
    await vi.advanceTimersByTimeAsync(60);
    await p;

    expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: 'center' });
    expect(document.activeElement).toBe(row);
    expect(row.classList.contains('ep-flash')).toBe(true);
  });

  it('bails cleanly when the row never appears', async () => {
    vi.useFakeTimers();
    const p = jumpToEndpoint('no-such-row');
    // Well past the full retry window (~8 x 50ms).
    await vi.advanceTimersByTimeAsync(1000);
    await p;
    expect(Element.prototype.scrollIntoView).not.toHaveBeenCalled();
  });

  it('moves the flash from an old target to a new one on a second jump', async () => {
    const aRaw = 'ep-a-raw';
    const bRaw = 'ep-b-raw';
    const rowA = document.createElement('tr');
    rowA.id = `ep-${stableId(aRaw)}`;
    rowA.tabIndex = -1;
    const rowB = document.createElement('tr');
    rowB.id = `ep-${stableId(bRaw)}`;
    rowB.tabIndex = -1;
    document.body.append(rowA, rowB);

    await jumpToEndpoint(aRaw);
    expect(rowA.classList.contains('ep-flash')).toBe(true);
    expect(rowB.classList.contains('ep-flash')).toBe(false);

    await jumpToEndpoint(bRaw);
    expect(rowA.classList.contains('ep-flash')).toBe(false);
    expect(rowB.classList.contains('ep-flash')).toBe(true);
  });
});

describe('jumpToEndpointKey (URL restore path)', () => {
  it('reveals the row from its stableId without re-pushing the hash', async () => {
    const rawId = 'restore-id';
    const key = stableId(rawId);
    const domId = `ep-${key}`;
    // Pre-seed the hash the way App's restore would: the entry is already on
    // the stack, so the helper must not push again.
    history.replaceState(null, '', `#endpoints/${domId}`);
    const row = document.createElement('tr');
    row.id = domId;
    row.tabIndex = -1;
    document.body.append(row);

    const before = location.hash;
    await jumpToEndpointKey(key);
    expect(navigation.current).toBe('endpoints');
    expect(location.hash).toBe(before);
    expect(document.activeElement).toBe(row);
    expect(row.classList.contains('ep-flash')).toBe(true);
  });
});
