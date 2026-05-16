// Config state — fetched once on app boot to learn which optional features
// (Google Photos, attachments) the backend has enabled. Frontend uses the
// flags to decide whether to render the relevant UI.

import { apiFetch } from '$lib/api';

export type AppConfig = {
	version: string;
	attachmentsEnabled: boolean;
	googlePhotosEnabled: boolean;
	maxAttachmentBytes: number;
};

class ConfigState {
	config = $state<AppConfig | null>(null);
	loading = $state(false);
	error = $state<string | null>(null);
	private pending: Promise<AppConfig> | null = null;

	async load(): Promise<AppConfig> {
		if (this.config) return this.config;
		if (this.pending) return this.pending;
		this.loading = true;
		this.pending = apiFetch<AppConfig>('/v1/config')
			.then((c) => {
				this.config = c;
				return c;
			})
			.catch((e: unknown) => {
				this.error = e instanceof Error ? e.message : String(e);
				// Safe defaults — features stay off until the backend speaks.
				const fallback: AppConfig = {
					version: 'unknown',
					attachmentsEnabled: false,
					googlePhotosEnabled: false,
					maxAttachmentBytes: 25 * 1024 * 1024
				};
				this.config = fallback;
				return fallback;
			})
			.finally(() => {
				this.loading = false;
				this.pending = null;
			});
		return this.pending;
	}
}

export const appConfig = new ConfigState();
