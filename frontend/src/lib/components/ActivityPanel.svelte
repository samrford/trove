<script lang="ts">
	import { listActivity } from '$lib/api/activity';
	import { collapseBursts, isBurst, matchesFilter, type ActivityRow } from '$lib/activity';
	import { SvelteSet } from 'svelte/reactivity';
	import { errMsg } from '$lib/api';
	import { relativeTime } from '$lib/time';
	import ActivityEntry from './ActivityEntry.svelte';
	import { fly } from 'svelte/transition';
	import { X, ChevronDown, ChevronRight, ArrowRight } from '@lucide/svelte';

	// Slide-in browse surface (mirrors QuickViewPanel's slide-in). ~20 recent, burst-collapsed.

	type Props = {
		open?: boolean;
		slug: string;
		refreshKey?: unknown;
	};
	let { open = $bindable(false), slug, refreshKey }: Props = $props();

	let rows = $state<ActivityRow[] | null>(null);
	let error = $state<string | null>(null);
	let expanded = new SvelteSet<string>();
	// Driven by SSE in the next chunk; inert for now.
	const newCount = $state(0);

	function burstKey(r: Extract<ActivityRow, { kind: 'burst' }>): string {
		return r.entries[0].id;
	}
	function toggle(key: string) {
		if (expanded.has(key)) expanded.delete(key);
		else expanded.add(key);
	}

	$effect(() => {
		void refreshKey;
		if (!open) return;
		const s = slug;
		if (!s) return;
		let cancelled = false;
		rows = null;
		error = null;
		listActivity(s, { limit: 20 })
			.then((page) => {
				if (!cancelled)
					rows = collapseBursts(
						page.activity.filter((e) => matchesFilter(e, { includeReorders: false }))
					);
			})
			.catch((e) => {
				if (!cancelled) error = errMsg(e);
			});
		return () => {
			cancelled = true;
		};
	});

	$effect(() => {
		if (!open) return;
		function onKey(e: KeyboardEvent) {
			if (e.key === 'Escape') open = false;
		}
		document.addEventListener('keydown', onKey);
		return () => document.removeEventListener('keydown', onKey);
	});
</script>

{#if open}
	<button
		type="button"
		aria-label="Close activity"
		onclick={() => (open = false)}
		class="fixed inset-0 z-30 bg-black/40 backdrop-blur-sm"
	></button>

	<aside
		transition:fly={{ x: 400, duration: 200 }}
		class="fixed inset-y-0 right-0 z-40 flex w-full max-w-md flex-col border-l border-line bg-card shadow-2xl"
	>
		<header class="flex items-center justify-between border-b border-line px-4 py-3">
			<div class="flex items-center gap-2">
				<h2 class="text-sm font-medium text-fg">Activity</h2>
				{#if newCount > 0}
					<span class="inline-flex items-center gap-1 text-xs text-accent">
						<span class="h-1.5 w-1.5 rounded-full bg-accent"></span>
						{newCount} new
					</span>
				{/if}
			</div>
			<button
				type="button"
				onclick={() => (open = false)}
				aria-label="Close"
				title="Close"
				class="rounded-md p-1.5 text-fg-muted transition hover:bg-card-2 hover:text-fg"
			>
				<X class="h-4 w-4" />
			</button>
		</header>

		<div class="flex-1 overflow-y-auto px-4 py-3">
			{#if error}
				<p class="text-xs text-danger">{error}</p>
			{:else if rows === null}
				<p class="text-xs text-fg-faint">Loading…</p>
			{:else if rows.length === 0}
				<p class="text-xs text-fg-faint">Nothing's happened here yet.</p>
			{:else}
				<ul class="flex flex-col gap-2.5">
					{#each rows as row (isBurst(row) ? burstKey(row) : row.id)}
						{#if isBurst(row)}
							{@const key = burstKey(row)}
							<li>
								<button
									type="button"
									onclick={() => toggle(key)}
									class="flex w-full items-center gap-1.5 text-left text-sm text-fg-muted transition hover:text-fg"
								>
									{#if expanded.has(key)}
										<ChevronDown class="h-3.5 w-3.5 shrink-0" />
									{:else}
										<ChevronRight class="h-3.5 w-3.5 shrink-0" />
									{/if}
									<span class="flex-1">You made {row.entries.length} changes</span>
									<span class="text-xs text-fg-faint">{relativeTime(row.latest)}</span>
								</button>
								{#if expanded.has(key)}
									<ul class="mt-2 flex flex-col gap-2 border-l border-line pl-3">
										{#each row.entries as entry (entry.id)}
											<li><ActivityEntry {entry} density="compact" showItemRef /></li>
										{/each}
									</ul>
								{/if}
							</li>
						{:else}
							<li><ActivityEntry entry={row} density="compact" showItemRef /></li>
						{/if}
					{/each}
				</ul>
			{/if}
		</div>

		<footer class="border-t border-line px-4 py-3">
			<a
				href={`/projects/${slug}/activity`}
				onclick={() => (open = false)}
				class="inline-flex items-center gap-1.5 text-xs text-fg-muted transition hover:text-fg hover:underline"
			>
				View full history
				<ArrowRight class="h-3.5 w-3.5" />
			</a>
		</footer>
	</aside>
{/if}
