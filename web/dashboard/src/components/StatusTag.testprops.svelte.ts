// Test-only reactive props holder: a .svelte.ts module is the way to get a
// $state object that mount() will track, so a test can change `issues`
// after mount.
export const tagProps: { status: string; issues: string[] } = $state({ status: 'offline', issues: ['unavailable'] });
