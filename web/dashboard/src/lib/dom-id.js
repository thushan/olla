// Deterministic, DOM-safe id derived from an exact string.
//
// The old cssId() slug stripped punctuation (`[^a-z0-9]+` -> `-`), so two
// distinct names that differ only in punctuation ("node.a" vs "node-a")
// collapsed to the same DOM id. getElementById resolves to whichever row
// rendered first, so "jump to endpoint" silently scrolled to the wrong row.
//
// This hash (FNV-1a 32-bit -> base36) is collision-resistant, not lossless -
// a 32-bit space can theoretically map two distinct strings to the same id,
// but at real fleet sizes (tens to low hundreds of endpoints/models) that is
// negligible. It emits only [0-9a-z] so the result is always a legal HTML id,
// and is a pure function of the input, so OverviewPanel's jump lookup and
// EndpointsPanel's row id agree without either panel needing to know the
// other's sort order.
export function stableId(str) {
  let h = 0x811c9dc5;
  const s = String(str ?? '');
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return (h >>> 0).toString(36);
}
