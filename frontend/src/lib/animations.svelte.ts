// User preference for in-app animations. When `enabled` is false, transitions
// like the QuickView content-swap flip are skipped — content just replaces in
// place. Stored in localStorage, defaults to enabled. Also respects the OS
// `prefers-reduced-motion` setting on first load (overridable by the toggle).

const STORAGE_KEY = 'trove-animations-enabled';

class AnimationPrefs {
	enabled = $state(true);

	constructor() {
		if (typeof window === 'undefined') return;

		try {
			const stored = localStorage.getItem(STORAGE_KEY);
			if (stored === 'true' || stored === 'false') {
				this.enabled = stored === 'true';
				return;
			}
		} catch {
			// localStorage unavailable — fall through to the OS preference.
		}

		// No explicit pref yet: defer to the OS reduced-motion setting.
		try {
			this.enabled = !window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		} catch {
			this.enabled = true;
		}
	}

	toggle() {
		this.enabled = !this.enabled;
		try {
			localStorage.setItem(STORAGE_KEY, String(this.enabled));
		} catch {
			// localStorage unavailable — preference still applies for this session.
		}
	}
}

export const animations = new AnimationPrefs();
