/// <reference types="vite/client" />

// The dashboard ships as a browser SPA, but its component tests run under
// vitest (Node + jsdom). The DOM lib covers `document`/`window`, but the
// Node `global` object the tests assign `global.fetch` on is not declared by
// the DOM lib and `@types/node` is intentionally not a dependency. When the
// test files were `.js` with checkJs off, TypeScript's JS loose mode inferred
// `global` as `any` implicitly; the migration to `.ts` removed that inference.
// Declared `any` here to preserve that behaviour: every fetch mock returns a
// partial `{ status, ok, headers, json }` shape that the store's `as T` cast
// narrows at the boundary, so strict `Response` typing on `global.fetch`
// would force rewriting every mock without any safety payoff.
declare const global: any;
