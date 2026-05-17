<script lang="ts">
	import { listActivity } from '$lib/api/activity';
	import { matchesFilter, type Activity } from '$lib/activity';
	import { errMsg } from '$lib/api';
	import ActivityEntry from './ActivityEntry.svelte';

	type Props = {
		slug: string;
		itemId: string;
		limit?: number;
		refreshKey?: unknown;
	};
	let { slug, itemId, limit = 50, refreshKey }: Props = $props();

	let entries = $state<Activity[] | null>(null);
	let error = $state<string | null>(null);

	$effect(() => {
		void refreshKey; // track so a parent bump refetches
		const s = slug;
		const id = itemId;
		if (!s || !id) return;
		let cancelled = false;
		entries = null;
		error = null;
		// Over-fetch past `limit` so dropping reorders client-side still yields
		// up to `limit` real events (mirrors ActivityStrip's buffer).
		listActivity(s, { itemId: id, limit: Math.max(limit * 4, 20) })
			.then((page) => {
				if (cancelled) return;
				entries = page.activity
					.filter((e) => matchesFilter(e, { includeReorders: false }))
					.slice(0, limit);
			})
			.catch((e) => {
				if (!cancelled) error = errMsg(e);
			});
		return () => {
			cancelled = true;
		};
	});
</script>

{#if error}
	<p class="text-xs text-danger">{error}</p>
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
