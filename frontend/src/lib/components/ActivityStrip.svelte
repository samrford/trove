<script lang="ts">
	import { listActivity } from '$lib/api/activity';
	import type { Activity } from '$lib/activity';
	import { realtime } from '$lib/realtime.svelte';
	import { createLiveFeed, keepRealFor } from '$lib/liveActivityFeed.svelte';
	import type { Project } from '$lib/api/projects';
	import ActivityEntry from './ActivityEntry.svelte';
	import { ChevronRight } from '@lucide/svelte';

	// Ambient strip on the project page: the latest few real events + a CTA
	// into the panel. Live via the realtime store; the catch-up fetch covers
	// initial render + reconnect-resync.

	type Props = {
		project: Pick<Project, 'id' | 'slug'>;
		onOpenPanel: () => void;
	};
	let { project, onOpenPanel }: Props = $props();

	const SHOWN = 3;

	// Over-fetch from the server so dropping reorders from the catch-up
	// snapshot still yields a populated strip.
	const feed = createLiveFeed({
		fetch: () => listActivity(project.slug, { limit: 20 }),
		keep: keepRealFor((a: Activity) => a.project_id === project.id)
	});

	const entries = $derived(feed.entries === null ? null : feed.entries.slice(0, SHOWN));

	$effect(() => {
		if (!project.slug || !project.id) return;
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
		{#if feed.error}
			<p class="text-xs text-danger">{feed.error}</p>
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
