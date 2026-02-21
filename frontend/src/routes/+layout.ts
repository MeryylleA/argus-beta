// Force pure client-side SPA — no SSR, no prerendering of individual routes.
// The adapter-static fallback: 'index.html' handles all routing client-side.
export const prerender = true;
export const ssr = false;
