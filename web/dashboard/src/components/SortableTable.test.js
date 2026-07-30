import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach } from 'vitest';
import Fixture from './SortableTable.grouped-fixture.svelte';

// Regression coverage for finding 8: the Models panel's grouped view passed
// rows={[]} to SortableTable and rendered its own closure-local, never-sorted
// row list in groupSnippet. Clicking a header flipped the indicator and
// aria-sort, but the DOM order never moved - also an a11y defect, since
// aria-sort then announced a state the rows didn't reflect.

const columns = [
  { key: 'name', label: 'Name', sortable: true },
  { key: 'n', label: 'Count', sortable: true, num: true },
];

// Values chosen so lexicographic string sort ("100" < "2" < "9") and numeric
// sort (2 < 9 < 100) disagree - this is what catches a sort that silently
// fell back to comparing formatted strings.
const groups = [
  {
    id: 'alpha',
    rows: [
      { id: 'a-100', name: 'a-100', n: 100 },
      { id: 'a-9', name: 'a-9', n: 9 },
      { id: 'a-2', name: 'a-2', n: 2 },
    ],
  },
  {
    id: 'beta',
    rows: [
      { id: 'b-50', name: 'b-50', n: 50 },
      { id: 'b-1', name: 'b-1', n: 1 },
    ],
  },
];

let component;

afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
});

function rowIdsFor(group) {
  return Array.from(document.querySelectorAll(`tr[data-testid="row"][data-group="${group}"]`)).map(
    (el) => el.dataset.id
  );
}

describe('SortableTable grouped sorting', () => {
  it('clicking the numeric column header reorders rows within every group, numerically', () => {
    component = mount(Fixture, { target: document.body, props: { columns, groups } });
    flushSync();

    // Unsorted: fixture order as given.
    expect(rowIdsFor('alpha')).toEqual(['a-100', 'a-9', 'a-2']);

    const header = Array.from(document.querySelectorAll('button.sort-btn')).find((b) =>
      b.textContent.includes('Count')
    );
    header.click();
    flushSync();

    // First click sorts descending (SortableTable's toggle() default).
    expect(rowIdsFor('alpha')).toEqual(['a-100', 'a-9', 'a-2']);
    expect(rowIdsFor('beta')).toEqual(['b-50', 'b-1']);

    header.click();
    flushSync();

    // Second click flips to ascending - numerically, not lexicographically
    // (a lexicographic sort would put "100" before "2" and "9").
    expect(rowIdsFor('alpha')).toEqual(['a-2', 'a-9', 'a-100']);
    expect(rowIdsFor('beta')).toEqual(['b-1', 'b-50']);
  });

  it('aria-sort on the header matches the order actually rendered', () => {
    component = mount(Fixture, { target: document.body, props: { columns, groups } });
    flushSync();

    const th = Array.from(document.querySelectorAll('th[aria-sort]')).find((t) =>
      t.textContent.includes('Count')
    );
    const header = th.querySelector('button.sort-btn');

    header.click();
    flushSync();
    expect(th.getAttribute('aria-sort')).toBe('descending');
    expect(rowIdsFor('beta')).toEqual(['b-50', 'b-1']); // descending

    header.click();
    flushSync();
    expect(th.getAttribute('aria-sort')).toBe('ascending');
    expect(rowIdsFor('beta')).toEqual(['b-1', 'b-50']); // ascending
  });
});
