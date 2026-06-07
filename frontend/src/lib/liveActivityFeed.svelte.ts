// Catch-up + live activity feed primitive shared by the activity surfaces
// (Rail / Strip / Panel — and structurally similar to the dedicated activity
// page, which has enough extra shape — load-more, next cursor, filter groups
// — that it stays inline).
//
// Why this exists: every surface needs the same race-safe choreography:
//   - Initial fetch + a `resync`-triggered refetch.
//   - Live `activity.added` events prepended via applyActivityFeed (which
//     dedupes by id and caps length).
//   - An event arriving while the fetch is in flight must NOT be overwritten
//     by the fetch result — it lands in a pending buffer and is drained into
//     the page on resolve.
//   - Concurrent fetches (e.g. resync mid-load) are superseded via a gen
//     counter: only the latest fetch's resolve commits.
//
// Components own SSE subscription (so they can branch on open/closed, gate
// newCount, etc.) and call `load()`/`ingest()`/`invalidate()` directly.

import { untrack } from 'svelte';
import { applyActivityFeed } from './realtime';
import { matchesFilter, type Activity } from './activity';
import { errMsg } from './api';

export type LiveFeedConfig = {
	/** Catch-up fetch. Called on load() (initial + every resync). */
	fetch: () => Promise<{ activity: Activity[] }>;
	/**
	 * Pre-filter applied to both catch-up entries and live ingest. Typical use:
	 * `(a) => a.item_id === id && matchesFilter(a, { includeReorders: false })`.
	 * Defaults to keeping everything.
	 */
	keep?: (a: Activity) => boolean;
	/** applyActivityFeed cap (default 200). Pass Infinity for the dedicated page. */
	cap?: number;
};

export class LiveFeed {
	entries = $state<Activity[] | null>(null);
	error = $state<string | null>(null);
	inFlight = $state(false);

	private pending: Activity[] = [];
	private fetchGen = 0;
	private readonly config: LiveFeedConfig;

	constructor(config: LiveFeedConfig) {
		this.config = config;
	}

	/** True if the event passes the configured `keep` filter. */
	keep(a: Activity): boolean {
		return this.config.keep ? this.config.keep(a) : true;
	}

	/** Kick off a fresh catch-up fetch. Supersedes any in-flight load. */
	load(): void {
		const myGen = ++this.fetchGen;
		this.pending = [];
		this.inFlight = true;
		// Only show the loading state (and clear a prior error) on the first load.
		// A resync-driven reload keeps the already-shown entries on screen until
		// the refetch resolves — no blank "Loading…" flash — and a transient
		// failure retains the stale feed instead of wiping it to an error.
		// `untrack` is load-bearing: surfaces call load() from inside an $effect,
		// so a tracked read of `entries` here would loop forever (the .then
		// writes entries → effect retriggers → load() → reads entries → …).
		if (untrack(() => this.entries) === null) this.error = null;
		this.config
			.fetch()
			.then((page) => {
				if (myGen !== this.fetchGen) return;
				let merged = this.config.keep
					? page.activity.filter((e) => this.config.keep!(e))
					: page.activity;
				for (const a of this.pending) {
					merged = applyActivityFeed(
						merged,
						{ type: 'activity.added', activity: a, cursor: '' },
						this.config.cap
					);
				}
				this.pending = [];
				this.entries = merged;
				this.error = null;
				this.inFlight = false;
			})
			.catch((e) => {
				if (myGen !== this.fetchGen) return;
				// Keep stale entries visible on a failed reload; only surface the
				// error when there's nothing else to show.
				if (this.entries === null) this.error = errMsg(e);
				this.inFlight = false;
			});
	}

	/** Apply a live activity. Drops events failing `keep`; buffers during an
	 * in-flight fetch and drains on resolve. */
	ingest(activity: Activity): void {
		if (!this.keep(activity)) return;
		if (this.inFlight) {
			this.pending.push(activity);
			return;
		}
		this.entries = applyActivityFeed(
			this.entries ?? [],
			{ type: 'activity.added', activity, cursor: '' },
			this.config.cap
		);
	}

	/** Invalidate any in-flight fetch so its late resolve can't stamp stale
	 * data. Use in effect cleanups and on close-like state transitions. */
	invalidate(): void {
		this.fetchGen++;
		this.inFlight = false;
	}
}

export function createLiveFeed(config: LiveFeedConfig): LiveFeed {
	return new LiveFeed(config);
}

// Convenience: the keep filter the Rail / Strip / Panel all use — scope to a
// project or item, and drop reorder-only updates (those surfaces never show
// reorders by policy, and ingesting them would let a noisy reorder stream
// push real events out of applyActivityFeed's 200 cap).
export function keepRealFor(predicate: (a: Activity) => boolean): (a: Activity) => boolean {
	return (a) => predicate(a) && matchesFilter(a, { includeReorders: false });
}
