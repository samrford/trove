// Theme state — three-way preference (light / dark / system) backed by
// localStorage. The actual class on <html> is set by an inline script in
// app.html before any CSS loads (so there's no flash on first paint); this
// module keeps that class in sync with user changes and OS-level changes
// while in 'system' mode.

const STORAGE_KEY = 'trove-theme';

export type ThemePreference = 'light' | 'dark' | 'system';
export type ResolvedTheme = 'light' | 'dark';

class ThemeState {
	preference = $state<ThemePreference>('system');
	resolved = $state<ResolvedTheme>('light');

	constructor() {
		if (typeof window === 'undefined') return;

		try {
			const stored = localStorage.getItem(STORAGE_KEY);
			if (stored === 'light' || stored === 'dark' || stored === 'system') {
				this.preference = stored;
			}
		} catch {
			// localStorage unavailable — keep default 'system'.
		}

		this.applyTheme();

		// React to OS preference changes while in 'system' mode.
		const mq = window.matchMedia('(prefers-color-scheme: dark)');
		mq.addEventListener('change', () => {
			if (this.preference === 'system') this.applyTheme();
		});
	}

	setPreference(pref: ThemePreference) {
		this.preference = pref;
		try {
			localStorage.setItem(STORAGE_KEY, pref);
		} catch {
			// localStorage unavailable — preference still applies for this session.
		}
		this.applyTheme();
	}

	private applyTheme() {
		if (typeof document === 'undefined') return;
		const resolved: ResolvedTheme =
			this.preference === 'system'
				? window.matchMedia('(prefers-color-scheme: dark)').matches
					? 'dark'
					: 'light'
				: this.preference;
		this.resolved = resolved;
		document.documentElement.classList.remove('theme-light', 'theme-dark');
		document.documentElement.classList.add(`theme-${resolved}`);
	}
}

export const theme = new ThemeState();
