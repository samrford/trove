import { supabase } from '$lib/supabase';
import type { Session, User } from '@supabase/supabase-js';

// Browser-only auth state. Holds the current Supabase session + user, and
// exposes sign-in / sign-up / sign-out helpers. Pages and components read
// `auth.user`, `auth.session`, `auth.loading` reactively via runes.

class AuthState {
	session = $state<Session | null>(null);
	user = $state<User | null>(null);
	loading = $state(true);

	constructor() {
		// Hydrate from whatever storage the Supabase client is using
		// (cookies on subdomains, localStorage otherwise).
		supabase.auth.getSession().then(({ data }) => {
			this.session = data.session;
			this.user = data.session?.user ?? null;
			this.loading = false;
		});

		// Stay in sync on sign-in / sign-out / token refresh.
		supabase.auth.onAuthStateChange((_event, session) => {
			this.session = session;
			this.user = session?.user ?? null;
			this.loading = false;
		});
	}

	async signInWithProvider(provider: 'google' | 'github' | 'discord') {
		return supabase.auth.signInWithOAuth({
			provider,
			options: { redirectTo: window.location.origin }
		});
	}

	async signInWithPassword(email: string, password: string) {
		return supabase.auth.signInWithPassword({ email, password });
	}

	async signUpWithPassword(email: string, password: string) {
		return supabase.auth.signUp({ email, password });
	}

	async signOut() {
		return supabase.auth.signOut();
	}
}

export const auth = new AuthState();

// Display helpers — pull the friendliest value out of Supabase's user_metadata
// (populated by OAuth providers) with sensible fallbacks. Email/password users
// won't have a name or avatar, so we fall back to the email and an initial.

export function userDisplayName(user: User | null | undefined): string {
	if (!user) return '';
	const meta = user.user_metadata ?? {};
	return meta.full_name || meta.name || user.email || '';
}

export function userAvatarUrl(user: User | null | undefined): string | null {
	if (!user) return null;
	const meta = user.user_metadata ?? {};
	return meta.avatar_url || meta.picture || null;
}

export function userInitial(user: User | null | undefined): string {
	const name = userDisplayName(user);
	return name.charAt(0).toUpperCase() || '?';
}
