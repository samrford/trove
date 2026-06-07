<script lang="ts">
	import { listActivity } from '$lib/api/activity';
	import type { Activity } from '$lib/activity';
	import { realtime } from '$lib/realtime.svelte';
	import { createLiveFeed, keepRealFor } from '$lib/liveActivityFeed.svelte';
	import ActivityEntry from './ActivityEntry.svelte';

	type Props = {
		slug: string;
		itemId: string;
		limit?: number;
	};
	let { slug, itemId, limit = 50 }: Props = $props();

	// Buffer holds *real* events only — the `keep` filter drops reorders at
	// ingress (catch-up + live), so a noisy live reorder stream can't push
	// real events out of applyActivityFeed's 200 cap.
	const feed = createLiveFeed({
		// Over-fetch past `limit` so dropping reorders from the catch-up
		// snapshot still yields up to `limit` real events.
		fetch: () => listActivity(slug, { itemId, limit: Math.max(limit * 4, 20) }),
		keep: keepRealFor((a: Activity) => a.item_id === itemId)
	});

	const entries = $derived(feed.entries === null ? null : feed.entries.slice(0, limit));

	$effect(() => {
		if (!slug || !itemId) return;
		feed.load();
		const unsubAdded = realtime.on('activity.added', (ev) => feed.ingest(ev.activity));
		const unsubResync = realtime.on('resync', () => feed.load());
		return () => {
			feed.invalidate();
			unsubAdded();
			unsubResync();
		};
	});
</script>

{#if feed.error}
	<p class="text-xs text-danger">{feed.error}</p>
{:else if entries === null}
	<p class="text-xs text-fg-faint">Loading…</p>
{:else if entries.length === 0}
	<p class="text-xs text-fg-faint">Nothing's happened here yet.</p>
{:else}
	<ul class="flex flex-col gap-2">
		{#each entries as entry (entry.id)}
			<li><ActivityEntry {entry} density="rail" /></li>
		{/each}
	</ul>
{/if}
