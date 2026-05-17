<script lang="ts">
	import { listActivity } from '$lib/api/activity';
	import { matchesFilter, type Activity } from '$lib/activity';
	import { errMsg } from '$lib/api';
	import ActivityEntry from './ActivityEntry.svelte';
	import { ChevronRight } from '@lucide/svelte';

	// Ambient strip on the project page: the latest few real events + a CTA
	// into the panel.

	type Props = {
		slug: string;
		onOpenPanel: () => void;
		refreshKey?: unknown;
	};
	let { slug, onOpenPanel, refreshKey }: Props = $props();

	const SHOWN = 3;

	let entries = $state<Activity[] | null>(null);
	let error = $state<string | null>(null);

	$effect(() => {
		void refreshKey;
		const s = slug;
		if (!s) return;
		let cancelled = false;
		// Over-fetch then drop reorders client-side so we still get shown real ones.
		listActivity(s, { limit: 20 })
			.then((page) => {
				if (cancelled) return;
				entries = page.activity
					.filter((e) => matchesFilter(e, { includeReorders: false }))
					.slice(0, SHOWN);
			})
			.catch((e) => {
				if (!cancelled) error = errMsg(e);
			});
		return () => {
			cancelled = true;
		};
	});
</script>

<section class="rounded-lg border border-line bg-card">
	<header class="flex items-center justify-between border-b border-line px-4 py-2">
		<h2 class="text-sm font-medium text-fg">Recent activity</h2>
		<button
			type="button"
			onclick={onOpenPanel}
			class="inline-flex items-center gap-0.5 text-xs text-fg-muted transition hover:text-fg"
		>
			View all
			<ChevronRight class="h-3.5 w-3.5" />
		</button>
	</header>
	<div class="px-4 py-3">
		{#if error}
			<p class="text-xs text-danger">{error}</p>
		{:else if entries === null}
			<p class="text-xs text-fg-faint">Loading…</p>
		{:else if entries.length === 0}
			<p class="text-xs text-fg-faint">Nothing's happened here yet.</p>
		{:else}
			<ul class="flex flex-col gap-2">
				{#each entries as entry (entry.id)}
					<li><ActivityEntry {entry} density="compact" showItemRef /></li>
				{/each}
			</ul>
		{/if}
	</div>
</section>
