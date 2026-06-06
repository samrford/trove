<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getProject, type Project } from '$lib/api/projects';
	import { listItems, type Item } from '$lib/api/items';
	import { listActivity, type ActivityCursor } from '$lib/api/activity';
	import {
		groupByDay,
		actionsForGroups,
		matchesFilter,
		ACTION_GROUPS,
		type Activity
	} from '$lib/activity';
	import { errMsg } from '$lib/api';
	import { realtime } from '$lib/realtime.svelte';
	import { applyActivityFeed } from '$lib/realtime';
	import ActivityEntry from '$lib/components/ActivityEntry.svelte';
	import { ArrowLeft } from '@lucide/svelte';

	const PAGE_SIZE = 50;

	let project = $state<Project | null>(null);
	let items = $state<Item[]>([]);
	let loadError = $state<string | null>(null);

	let entries = $state<Activity[]>([]);
	let nextCursor = $state<ActivityCursor | null>(null);
	let loading = $state(false);
	let loadingMore = $state(false);
	let feedError = $state<string | null>(null);
	// Bumped on every page-1 (filter-change) refetch so an in-flight loadMore
	// can detect its results are stale and discard them. Deliberately a plain
	// `let` (not $state): tracking it reactively would self-trigger the effect.
	let feedGen = 0;

	// Filters. Action groups + item + actor are server-side; reorders is a
	// client-side toggle (a position-only item.updated isn't a distinct action).
	let selectedGroups = $state<string[]>([]);
	let itemFilter = $state('');
	let actorFilter = $state(''); // '' = anyone | 'me' = current user
	let includeReorders = $state(false);

	$effect(() => {
		if (!auth.loading && !auth.user) goto('/login');
	});

	let loadedSlug = $state<string | null>(null);

	// Load project + item list when the slug changes — covers client-side nav
	// between different projects' activity pages, not just first mount.
	$effect(() => {
		const slug = page.params.slug;
		if (!auth.user || !slug || slug === loadedSlug) return;
		loadedSlug = slug;
		project = null;
		items = [];
		Promise.all([getProject(slug), listItems(slug)])
			.then(([p, its]) => {
				project = p;
				items = its;
			})
			.catch((e) => (loadError = errMsg(e)));
	});

	function fetchOpts(cursor: ActivityCursor | null) {
		const actions = actionsForGroups(selectedGroups);
		return {
			actions: actions.length > 0 ? actions : undefined,
			itemId: itemFilter || undefined,
			actorId: actorFilter === 'me' ? (auth.user?.id ?? undefined) : undefined,
			limit: PAGE_SIZE,
			cursor
		};
	}

	// Live events arriving during a page-1 fetch — drained on resolve so a
	// strictly-newer live event isn't overwritten by the fetch result. Used by
	// both the filter-change effect below and the resync handler.
	let pending: Activity[] = [];

	function fetchPage1(isCancelled: () => boolean = () => false): void {
		const slug = page.params.slug;
		if (!slug) return;
		const myGen = ++feedGen;
		pending = [];
		loading = true;
		feedError = null;
		listActivity(slug, fetchOpts(null))
			.then((p) => {
				if (isCancelled() || myGen !== feedGen) return;
				let merged = p.activity;
				for (const a of pending) {
					merged = applyActivityFeed(
						merged,
						{ type: 'activity.added', activity: a, cursor: '' },
						Infinity
					);
				}
				pending = [];
				entries = merged;
				nextCursor = p.next;
			})
			.catch((e) => {
				if (isCancelled() || myGen !== feedGen) return;
				feedError = errMsg(e);
			})
			.finally(() => {
				if (!isCancelled() && myGen === feedGen) loading = false;
			});
	}

	// Refetch page 1 whenever a server-side filter changes.
	$effect(() => {
		void selectedGroups;
		void itemFilter;
		void actorFilter;
		if (!auth.user || !page.params.slug) return;
		let cancelled = false;
		fetchPage1(() => cancelled);
		return () => {
			cancelled = true;
		};
	});

	// Live subscription. Prepends activity.added events that match the active
	// server-side filters (action groups / item / actor); the reorders toggle
	// is client-side so live reorders flow into entries either way — the
	// derived `groups` decides whether to show them. The default 200 cap on
	// applyActivityFeed would chop off the tail a `loadMore`-deep user just
	// paginated to, so override to Infinity (the page is leaf-navigation —
	// it'll GC on navigation away).
	$effect(() => {
		if (!project) return;
		const projectId = project.id;

		function matchesPageFilters(a: Activity): boolean {
			if (a.project_id !== projectId) return false;
			if (selectedGroups.length > 0) {
				const allowed = actionsForGroups(selectedGroups);
				if (!allowed.includes(a.action)) return false;
			}
			if (itemFilter && a.item_id !== itemFilter) return false;
			if (actorFilter === 'me' && a.actor_id !== auth.user?.id) return false;
			return true;
		}

		const unsubAdded = realtime.on('activity.added', (ev) => {
			if (!matchesPageFilters(ev.activity)) return;
			if (loading) {
				// Page-1 fetch in flight — buffer for the merge on resolve.
				pending.push(ev.activity);
				return;
			}
			entries = applyActivityFeed(entries, ev, Infinity);
		});
		const unsubResync = realtime.on('resync', () => fetchPage1());
		return () => {
			unsubAdded();
			unsubResync();
		};
	});

	async function loadMore() {
		const slug = page.params.slug;
		if (!slug || !nextCursor || loadingMore) return;
		const gen = feedGen;
		loadingMore = true;
		try {
			const p = await listActivity(slug, fetchOpts(nextCursor));
			if (gen !== feedGen) return; // a filter changed mid-flight — discard
			entries = [...entries, ...p.activity];
			nextCursor = p.next;
		} catch (e) {
			if (gen === feedGen) feedError = errMsg(e);
		} finally {
			loadingMore = false;
		}
	}

	function toggleGroup(g: string) {
		selectedGroups = selectedGroups.includes(g)
			? selectedGroups.filter((x) => x !== g)
			: [...selectedGroups, g];
	}

	const groups = $derived(groupByDay(entries.filter((e) => matchesFilter(e, { includeReorders }))));
	const groupNames = Object.keys(ACTION_GROUPS);
</script>

{#if !auth.loading && auth.user}
	<main class="mx-auto max-w-3xl px-6 py-10">
		<div class="mb-6">
			<a
				href={project ? `/projects/${project.slug}` : '/'}
				class="inline-flex items-center gap-1.5 text-sm text-fg-muted hover:text-fg hover:underline"
			>
				<ArrowLeft class="h-4 w-4" />
				{project ? project.name : 'Back'}
			</a>
		</div>

		{#if loadError}
			<div class="rounded-md border border-danger/40 bg-danger/10 p-4">
				<p class="text-sm text-danger">{loadError}</p>
			</div>
		{:else}
			<header class="mb-6 border-b border-line pb-4">
				<h1 class="text-2xl font-semibold tracking-tight text-fg">Activity</h1>
			</header>

			<!-- Filters -->
			<div class="mb-5 flex flex-col gap-3">
				<div class="flex flex-wrap items-center gap-1.5">
					<button
						type="button"
						onclick={() => (selectedGroups = [])}
						class="rounded-full px-3 py-1 text-xs font-medium transition"
						class:bg-fg={selectedGroups.length === 0}
						class:text-on-accent={selectedGroups.length === 0}
						class:text-fg-muted={selectedGroups.length !== 0}
						class:hover:bg-card-2={selectedGroups.length !== 0}
					>
						All
					</button>
					{#each groupNames as g (g)}
						<button
							type="button"
							onclick={() => toggleGroup(g)}
							class="rounded-full px-3 py-1 text-xs font-medium transition"
							class:bg-fg={selectedGroups.includes(g)}
							class:text-on-accent={selectedGroups.includes(g)}
							class:text-fg-muted={!selectedGroups.includes(g)}
							class:hover:bg-card-2={!selectedGroups.includes(g)}
						>
							{g}
						</button>
					{/each}
				</div>
				<div class="flex flex-wrap items-center gap-3 text-xs">
					<select
						bind:value={itemFilter}
						aria-label="Filter by item"
						class="rounded-md border border-line bg-card px-2 py-1 text-fg"
					>
						<option value="">Any item</option>
						{#each items as it (it.id)}
							<option value={it.id}>#{it.sequence} {it.title}</option>
						{/each}
					</select>
					<select
						bind:value={actorFilter}
						aria-label="Filter by actor"
						class="rounded-md border border-line bg-card px-2 py-1 text-fg"
					>
						<option value="">Anyone</option>
						<option value="me">You</option>
					</select>
					<label class="inline-flex items-center gap-1.5 text-fg-muted">
						<input type="checkbox" bind:checked={includeReorders} />
						Show reorders
					</label>
				</div>
			</div>

			{#if feedError}
				<div
					class="mb-4 rounded-md border border-danger/40 bg-danger/10 px-3 py-2 text-sm text-danger"
				>
					{feedError}
				</div>
			{/if}

			{#if loading}
				<p class="text-sm text-fg-faint">Loading…</p>
			{:else}
				{#if groups.length === 0}
					<div class="rounded-lg border border-line bg-card p-8 text-center">
						<p class="text-sm text-fg-muted">
							{nextCursor
								? 'Nothing on this page matches — load more or widen the filters.'
								: 'Nothing matches — try widening the filters.'}
						</p>
					</div>
				{:else}
					{#each groups as group (group.label)}
						<section class="mb-6">
							<h2 class="mb-3 text-xs font-medium tracking-wide text-fg-faint uppercase">
								{group.label}
							</h2>
							<ul class="flex flex-col gap-3">
								{#each group.entries as entry (entry.id)}
									<li><ActivityEntry {entry} density="rich" showItemRef /></li>
								{/each}
							</ul>
						</section>
					{/each}
				{/if}

				{#if nextCursor}
					<div class="mt-4 flex justify-center">
						<button
							type="button"
							onclick={loadMore}
							disabled={loadingMore}
							class="rounded-md border border-line px-4 py-2 text-sm font-medium text-fg transition hover:bg-card-2 disabled:opacity-50"
						>
							{loadingMore ? 'Loading…' : 'Load more'}
						</button>
					</div>
				{/if}
			{/if}
		{/if}
	</main>
{/if}
