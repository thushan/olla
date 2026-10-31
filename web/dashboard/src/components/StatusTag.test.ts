import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';
import StatusTag from './StatusTag.svelte';
import { tagProps } from './StatusTag.testprops.svelte';

let component: ReturnType<typeof mount> | undefined;
beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
  vi.useRealTimers();
});

function setup() {
  component = mount(StatusTag, { target: document.body, props: { status: 'offline', issues: ['unavailable'] } });
  flushSync();
  const btn = document.querySelector<HTMLButtonElement>('button.status-tag')!;
  const tip = document.querySelector<HTMLElement>('[role="tooltip"]')!;
  return { btn, tip };
}

function geometry(btn: HTMLElement, tip: HTMLElement, rect: Partial<DOMRect>, tipH = 80) {
  btn.getBoundingClientRect = () => ({ top: 0, bottom: 0, left: 0, right: 0, width: 0, height: 0, x: 0, y: 0, toJSON() {}, ...rect }) as DOMRect;
  Object.defineProperty(tip, 'offsetHeight', { configurable: true, value: tipH });
}

describe('StatusTag tooltip placement', () => {
  it('sits below the pill when there is room', () => {
    const { btn, tip } = setup();
    window.innerWidth = 1000;
    window.innerHeight = 600;
    geometry(btn, tip, { top: 100, bottom: 120, left: 50 });
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();
    expect(tip.style.top).toBe('124px');
    expect(tip.style.left).toBe('50px');
  });

  it('flips above near the bottom edge and clamps to the right edge', () => {
    const { btn, tip } = setup();
    window.innerWidth = 390;
    window.innerHeight = 600;
    geometry(btn, tip, { top: 560, bottom: 580, left: 380 });
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();
    expect(tip.style.top).toBe('476px'); // 560 - 4 - 80
    expect(tip.style.left).toBe('142px'); // 390 - 240 - 8
  });
});

describe('StatusTag tooltip hover', () => {
  it('stays open while the pointer moves from the pill onto the tip, then closes', () => {
    const { btn, tip } = setup();
    geometry(btn, tip, { top: 10, bottom: 30, left: 10 });
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();
    expect(tip.classList.contains('open')).toBe(true);
    btn.dispatchEvent(new MouseEvent('mouseleave'));
    vi.advanceTimersByTime(60);
    tip.dispatchEvent(new MouseEvent('mouseenter'));
    vi.advanceTimersByTime(500);
    flushSync();
    expect(tip.classList.contains('open')).toBe(true);
    tip.dispatchEvent(new MouseEvent('mouseleave'));
    vi.advanceTimersByTime(200);
    flushSync();
    expect(tip.classList.contains('open')).toBe(false);
  });

  it('Escape closes it while another control has focus', () => {
    const { btn, tip } = setup();
    const other = document.createElement('input');
    document.body.appendChild(other);
    other.focus();
    geometry(btn, tip, { top: 10, bottom: 30, left: 10 });
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();
    expect(tip.classList.contains('open')).toBe(true);
    other.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    flushSync();
    expect(tip.classList.contains('open')).toBe(false);
  });

  it('Escape closes it immediately', () => {
    const { btn, tip } = setup();
    geometry(btn, tip, { top: 10, bottom: 30, left: 10 });
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();
    btn.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    flushSync();
    expect(tip.classList.contains('open')).toBe(false);
  });
});

describe('StatusTag portal and reset', () => {
  it('parents the tooltip on body, still described by the pill', () => {
    // Nested mount: without use:portal the tip would stay inside the wrapper.
    const wrap = document.createElement('div');
    wrap.id = 'wrap';
    document.body.appendChild(wrap);
    component = mount(StatusTag, { target: wrap, props: { status: 'offline', issues: ['unavailable'] } });
    flushSync();
    const btn = wrap.querySelector<HTMLButtonElement>('button.status-tag')!;
    const tip = document.querySelector<HTMLElement>('[role="tooltip"]')!;
    expect(btn.parentElement).toBe(wrap);
    expect(tip.parentElement).toBe(document.body);
    expect(document.getElementById(btn.getAttribute('aria-describedby')!)).toBe(tip);
  });

  it('does not reopen after issues clear and return', () => {
    tagProps.issues = ['unavailable'];
    component = mount(StatusTag, { target: document.body, props: tagProps });
    flushSync();
    let btn = document.querySelector<HTMLButtonElement>('button.status-tag')!;
    let tip = document.querySelector<HTMLElement>('[role="tooltip"]')!;
    btn.getBoundingClientRect = () => ({ top: 0, bottom: 10, left: 0, right: 10, width: 10, height: 10, x: 0, y: 0, toJSON() {} }) as DOMRect;
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    btn.dispatchEvent(new FocusEvent('focus'));
    flushSync();
    expect(tip.classList.contains('open')).toBe(true);
    tagProps.issues = [];
    flushSync();
    expect(document.querySelector('[role="tooltip"]')).toBeNull();
    tagProps.issues = ['unavailable'];
    flushSync();
    tip = document.querySelector<HTMLElement>('[role="tooltip"]')!;
    expect(tip.classList.contains('open')).toBe(false);
  });
});
