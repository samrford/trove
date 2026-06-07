// Realtime store — the single long-lived SSE connection. Owned at the app
// shell (mounted in +layout.svelte keyed on auth.user); surfaces subscribe
// via realtime.on(type, fn) and feed events into the pure reducers in
// $lib/realtime.ts. Decision-making lives in the reducers; this file is just
// connection lifecycle and dispatch.
//
// Auth: The existing AuthMiddleware authenticates
// /v1/events unchanged. Token rotates ~hourly via Supabase autoRefreshToken;
// we tear down + reconnect on TOKEN_REFRESHED so reconnects never 401.
//
// Catch-up: fetch-event-source automatically tracks SSE `id:` and resends
// Last-Event-ID on reconnect → server-side reconnect catch-up is automatic.

import { fetchEventSource, type EventSourceMessage } from '@microsoft/fetch-event-source';

import { supabase } from './supabase';
import { API_BASE_URL, getAccessToken } from './api';
import type { RealtimeEvent } from './realtime';
import type { Activity } from './activity';
import type { Item } from './api/items';
import type { Project } from './api/projects';

export type Status = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'error';

type AnyListener = (e: RealtimeEvent) => void;
type TypedListener<T extends RealtimeEvent['type']> = (
	e: Extract<RealtimeEvent, { type: T }>
) => void;

// Thrown from onopen on an auth failure (401/403) to stop fetch-event-source's
// retry loop — retrying would just reuse the same dead token. A token refresh is
// kicked off in parallel; its TOKEN_REFRESHED restarts the stream cleanly.
class FatalError extends Error {}

class RealtimeStore {
	status = $state<Status>('idle');

	private readonly listeners = new Map<RealtimeEvent['type'], Set<AnyListener>>();
	private abort: AbortController | null = null;
	private currentToken: string | null = null;
	private authSub: { unsubscribe: () => void } | null = null;

	constructor() {
		// Token rotation — re-handshake so future fetch-event-source reconnects
		// don't 401 with a stale token. (SIGNED_IN/SIGNED_OUT are driven by the
		// +layout.svelte $effect on the user id, not here.)
		const { data } = supabase.auth.onAuthStateChange((event, session) => {
			if (event !== 'TOKEN_REFRESHED') return;
			const token = session?.access_token ?? null;
			if (!token || !this.abort || token === this.currentToken) return;
			this.restart().catch((e) => {
				console.error('[realtime] restart failed', e);
				this.status = 'error';
			});
		});
		this.authSub = data.subscription;
	}

	// Tear down the auth subscription. Unused in production (the singleton lives
	// for the app's lifetime) but lets tests / future dispose paths clean up.
	dispose(): void {
		this.stop();
		this.authSub?.unsubscribe();
		this.authSub = null;
	}

	on<T extends RealtimeEvent['type']>(type: T, fn: TypedListener<T>): () => void {
		let set = this.listeners.get(type);
		if (!set) {
			// Plain Set: a non-reactive listener registry, never read in a $derived/$effect.
			// eslint-disable-next-line svelte/prefer-svelte-reactivity
			set = new Set();
			this.listeners.set(type, set);
		}
		set.add(fn as AnyListener);
		return () => set.delete(fn as AnyListener);
	}

	async start(): Promise<void> {
		if (this.abort) return; // already running
		const ac = new AbortController();
		this.abort = ac;
		this.status = 'connecting';

		const token = await getAccessToken();
		// stop/restart may have raced in while we awaited.
		if (!token || this.abort !== ac) {
			if (this.abort === ac) {
				this.abort = null;
				this.status = 'idle';
			}
			return;
		}
		this.currentToken = token;
		void this.runConnection(ac, token);
	}

	stop(): void {
		this.abort?.abort();
		this.abort = null;
		this.currentToken = null;
		this.status = 'idle';
	}

	private async restart(): Promise<void> {
		this.stop();
		await this.start();
	}

	private async runConnection(ac: AbortController, token: string): Promise<void> {
		try {
			await fetchEventSource(`${API_BASE_URL}/v1/events`, {
				headers: { Authorization: `Bearer ${token}` },
				signal: ac.signal,
				// Keep streaming when the tab is hidden so the user doesn't return
				// to a stale UI while a fresh connect catches up.
				openWhenHidden: true,
				onopen: async (res) => {
					if (res.ok) {
						this.status = 'connected';
						return;
					}
					if (res.status === 401 || res.status === 403) {
						// Stale/invalid token. fetch-event-source would otherwise
						// retry forever reusing this same dead token, so force a
						// refresh (fires TOKEN_REFRESHED → restart with a fresh
						// token) and stop this attempt with a fatal error.
						void supabase.auth.refreshSession();
						throw new FatalError(`SSE auth ${res.status}`);
					}
					// Transient (e.g. 5xx) — let the lib retry on its own backoff.
					throw new Error(`SSE open ${res.status}`);
				},
				onmessage: (msg) => this.dispatch(msg),
				onerror: (err) => {
					if (err instanceof FatalError) {
						this.status = 'error';
						throw err; // stop the retry loop; the token refresh restarts us
					}
					// Return nothing → fetch-event-source retries on its own backoff.
					// Throwing (other than FatalError) would stop retries.
					this.status = 'reconnecting';
				},
				onclose: () => {
					if (this.abort === ac) this.status = 'reconnecting';
				}
			});
		} catch {
			// Aborted (clean stop) or unrecoverable — settle whichever case applies.
			if (this.abort === ac) this.status = 'error';
		}
	}

	private dispatch(msg: EventSourceMessage): void {
		const cursor = msg.id ?? '';
		let event: RealtimeEvent | null = null;
		try {
			const data = msg.data ? JSON.parse(msg.data) : null;
			switch (msg.event) {
				case 'activity.added':
					event = { type: 'activity.added', activity: data as Activity, cursor };
					break;
				case 'item.changed':
					// Wire shape is { item, actor_id } so the actor rides with the
					// event (own-echo suppression compares it to the current user).
					event = {
						type: 'item.changed',
						item: data.item as Item,
						actorId: data.actor_id ?? null,
						cursor
					};
					break;
				case 'item.deleted':
					event = {
						type: 'item.deleted',
						payload: data,
						actorId: data?.actor_id ?? null,
						cursor
					};
					break;
				case 'project.changed':
					event = { type: 'project.changed', project: data as Project, cursor };
					break;
				case 'resync':
					event = { type: 'resync', cursor: '' };
					break;
			}
		} catch (e) {
			console.warn('[realtime] dropped malformed frame', { event: msg.event, error: e });
			return;
		}
		if (!event) return;
		const set = this.listeners.get(event.type);
		if (!set) return;
		// Snapshot first so a listener can safely unsubscribe mid-iteration; also
		// isolate a throwing listener so it can't kill the SSE connection (the
		// throw would propagate through fetch-event-source's onmessage).
		for (const l of set) {
			try {
				l(event);
			} catch (e) {
				console.error('[realtime] listener threw', { type: event.type, error: e });
			}
		}
	}
}

export const realtime = new RealtimeStore();
