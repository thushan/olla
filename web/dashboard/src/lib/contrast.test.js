import { describe, it, expect } from 'vitest';

// Token sets ported verbatim from src/app.css. If either changes, update the
// other to match. These are the actual colours shipped to operators.
const LIGHT = {
  bg: '#f1efe5',
  bgElevated: '#fbfaf4',
  bgInset: '#e7e4d6',
  bgHeader: '#e9e6d8',
  text: '#23261f',
  textDim: '#5c5a49',
  textFaint: '#6d6a58',
  accent: '#0d6b60',
  link: '#0d6b60',
  green: '#187338',
  greenBg: '#dcecdd',
  amber: '#8a5a00',
  amberBg: '#f1e4bf',
  red: '#a3241c',
  redBg: '#f3dcd8',
};
const DARK = {
  bg: '#0a0d0d',
  bgElevated: '#101414',
  bgInset: '#151a1a',
  bgHeader: '#0d1111',
  text: '#d9e4e1',
  textDim: '#8ba39d',
  textFaint: '#738581',
  accent: '#5fd9c6',
  link: '#7ee6d3',
  green: '#4fbd7a',
  greenBg: '#123322',
  amber: '#e0ac47',
  amberBg: '#3a2c0e',
  red: '#e5695f',
  redBg: '#3a1815',
};

function channel(hex) {
  const h = hex.replace('#', '');
  return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)];
}

function luminance(hex) {
  const [r, g, b] = channel(hex).map((v) => {
    const s = v / 255;
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function ratio(a, b) {
  const la = luminance(a);
  const lb = luminance(b);
  const [hi, lo] = la > lb ? [la, lb] : [lb, la];
  return (hi + 0.05) / (lo + 0.05);
}

const AA_NORMAL = 4.5; // body text here is 13px or smaller, so we hold everything to 4.5

describe('WCAG AA contrast', () => {
  describe('light theme', () => {
    const cases = [
      ['text on bg', LIGHT.text, LIGHT.bg],
      ['text on elevated', LIGHT.text, LIGHT.bgElevated],
      ['text on inset', LIGHT.text, LIGHT.bgInset],
      ['dim text on bg', LIGHT.textDim, LIGHT.bg],
      ['accent on bg', LIGHT.accent, LIGHT.bg],
      ['accent on elevated', LIGHT.accent, LIGHT.bgElevated],
      ['green status on bg', LIGHT.green, LIGHT.bg],
      ['amber status on bg', LIGHT.amber, LIGHT.bg],
      ['red status on bg', LIGHT.red, LIGHT.bg],
    ];
    for (const [name, fg, bg] of cases) {
      it(`${name} meets AA (${ratio(fg, bg).toFixed(2)}:1)`, () => {
        expect(ratio(fg, bg)).toBeGreaterThanOrEqual(AA_NORMAL);
      });
    }
    // text-faint is also used for normal-size prose (the footer line is
    // 11.2px/400), so it must clear the full 4.5:1 floor, not just the
    // large-text/UI threshold.
    it('faint text on bg meets AA for normal-size text (footer)', () => {
      expect(ratio(LIGHT.textFaint, LIGHT.bg)).toBeGreaterThanOrEqual(AA_NORMAL);
    });

    // Status tags render as coloured text on a tinted background of the same
    // hue, not on the page background, so that is the pairing that must pass.
    it('green status tag text on its tinted background meets AA', () => {
      expect(ratio(LIGHT.green, LIGHT.greenBg)).toBeGreaterThanOrEqual(AA_NORMAL);
    });
    it('amber status tag text on its tinted background meets AA', () => {
      expect(ratio(LIGHT.amber, LIGHT.amberBg)).toBeGreaterThanOrEqual(AA_NORMAL);
    });
    it('red status tag text on its tinted background meets AA', () => {
      expect(ratio(LIGHT.red, LIGHT.redBg)).toBeGreaterThanOrEqual(AA_NORMAL);
    });

    // Footer attribution link (item 13) renders on the footer's bg, which
    // is the page background, not a tinted surface.
    it('footer link on bg meets AA', () => {
      expect(ratio(LIGHT.link, LIGHT.bg)).toBeGreaterThanOrEqual(AA_NORMAL);
    });
  });

  describe('dark theme', () => {
    const cases = [
      ['text on bg', DARK.text, DARK.bg],
      ['text on elevated', DARK.text, DARK.bgElevated],
      ['text on inset', DARK.text, DARK.bgInset],
      ['dim text on bg', DARK.textDim, DARK.bg],
      ['accent on bg', DARK.accent, DARK.bg],
      ['accent on elevated', DARK.accent, DARK.bgElevated],
      ['green status on bg', DARK.green, DARK.bg],
      ['amber status on bg', DARK.amber, DARK.bg],
      ['red status on bg', DARK.red, DARK.bg],
    ];
    for (const [name, fg, bg] of cases) {
      it(`${name} meets AA (${ratio(fg, bg).toFixed(2)}:1)`, () => {
        expect(ratio(fg, bg)).toBeGreaterThanOrEqual(AA_NORMAL);
      });
    }
    it('faint text on bg meets AA for normal-size text (footer)', () => {
      expect(ratio(DARK.textFaint, DARK.bg)).toBeGreaterThanOrEqual(AA_NORMAL);
    });

    it('green status tag text on its tinted background meets AA', () => {
      expect(ratio(DARK.green, DARK.greenBg)).toBeGreaterThanOrEqual(AA_NORMAL);
    });
    it('amber status tag text on its tinted background meets AA', () => {
      expect(ratio(DARK.amber, DARK.amberBg)).toBeGreaterThanOrEqual(AA_NORMAL);
    });
    it('red status tag text on its tinted background meets AA', () => {
      expect(ratio(DARK.red, DARK.redBg)).toBeGreaterThanOrEqual(AA_NORMAL);
    });

    it('footer link on bg meets AA', () => {
      expect(ratio(DARK.link, DARK.bg)).toBeGreaterThanOrEqual(AA_NORMAL);
    });
  });
});
