// Auth lives entirely in the browser (Supabase JS client + cookie-storage in
// `$lib/supabase.ts`), so the SvelteKit server has no role in auth/session
// handling - clean separation between the SvelteKit UI and the Go API.
export const ssr = false;
