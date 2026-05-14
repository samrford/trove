// Sidebar collapse state — persists in localStorage so the user's preference
// survives reloads. Only affects desktop; on mobile the sidebar is a drawer
// with its own open/close state.

const STORAGE_KEY = 'trove-sidebar-collapsed';

class SidebarState {
	collapsed = $state<boolean>(false);

	constructor() {
		if (typeof window === 'undefined') return;
		try {
			this.collapsed = localStorage.getItem(STORAGE_KEY) === '1';
		} catch {
			// localStorage unavailable — keep default expanded.
		}
	}

	toggle() {
		this.collapsed = !this.collapsed;
		try {
			localStorage.setItem(STORAGE_KEY, this.collapsed ? '1' : '0');
		} catch {
			// localStorage unavailable — preference still applies for this session.
		}
	}
}

export const sidebar = new SidebarState();
