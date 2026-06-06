<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { getProject, deleteProject, type Project } from '$lib/api/projects';
	import {
		listItems,
		updateItem,
		detachTagFromItem,
		getItem,
		type Item,
		type ItemKind,
		type ItemStatus,
		type TagFilterMode,
		ITEM_KINDS
	} from '$lib/api/items';
	import { listTagsForProject, type TagWithCount } from '$lib/api/tags';
	import { ApiError, errMsg } from '$lib/api';
	import { realtime } from '$lib/realtime.svelte';
	import {
		applyEditorEvent,
		applyItemEvent,
		applyProjectEvent,
		type EditorAffordance
	} from '$lib/realtime';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import AddItemDialog from '$lib/components/AddItemDialog.svelte';
	import QuickViewPanel from '$lib/components/QuickViewPanel.svelte';
	import StatusIcon from '$lib/components/StatusIcon.svelte';
	import ItemTagChip from '$lib/components/ItemTagChip.svelte';
	import TagFilterPopover from '$lib/components/TagFilterPopover.svelte';
	import ActivityStrip from '$lib/components/ActivityStrip.svelte';
	import ActivityPanel from '$lib/components/ActivityPanel.svelte';
	import { projectColourVar } from '$lib/projectColours';
	import { KIND_LABEL, KIND_PLURAL, STATUS_LABEL, kindChipStyle } from '$lib/itemDisplay';
	import { relativeTime } from '$lib/time';
	import {
		Activity,
		ArrowLeft,
		Pencil,
		Trash2,
		Plus,
		ChevronDown,
		ChevronRight,
		X,
		Paperclip
	} from '@lucide/svelte';

	let project = $state<Project | null>(null);
	let items = $state<Item[] | null>(null);
	let tagsInProject = $state<TagWithCount[]>([]);
	let loadError = $state<string | null>(null);
	// Transient errors from item-level actions (drag, list ops). Distinct from
	// loadError so they don't masquerade as a project-load failure.
	let itemError = $state<string | null>(null);
	let kindFilter = $state<ItemKind | null>(null);
	let selectedTagSlugs = $state<string[]>([]);
	let tagMode = $state<TagFilterMode>('and');
	let showDone = $state(false);
	let showArchived = $state(false);
	let deleteConfirmOpen = $state(false);
	let deleting = $state(false);
	let addItemOpen = $state(false);
	let activityPanelOpen = $state(false);

	let draggingId = $state<string | null>(null);
	let dragOverStatus = $state<ItemStatus | null>(null);

	let quickViewItem = $state<Item | null>(null);
	let quickViewOpen = $state(false);
	// Bindable from QuickViewPanel so the parent listener knows whether to
	// silently sync the prop (clean) or surface an affordance (dirty editor).
	let quickViewEditing = $state(false);
	let quickViewAffordance = $state<EditorAffordance>('none');

	// Per-id cursor map for the items list, the project's own cursor, and a
	// dedicated cursor for the QuickView item — out-of-order safety lives in
	// applyItemEvent / applyProjectEvent / applyEditorEvent.
	let itemsLastSeen = $state<Record<string, string>>({});
	let projectLastSeen = $state('');
	let quickViewLastSeen = $state('');

	function openQuickView(item: Item) {
		quickViewItem = item;
		quickViewOpen = true;
		// Drop edit mode + any prior affordance — both referred to the previous item.
		quickViewEditing = false;
		quickViewAffordance = 'none';
		quickViewLastSeen = '';
	}

	function refreshTagsInProject() {
		if (!project) return;
		listTagsForProject(project.slug)
			.then((tgs) => (tagsInProject = tgs))
			.catch(() => {});
	}

	function sameTagSet(a: Item['tags'], b: Item['tags']) {
		if (a.length !== b.length) return false;
		const ids = new Set(a.map((t) => t.id));
		return b.every((t) => ids.has(t.id));
	}

	function handleItemUpdated(updated: Item) {
		const previous = items?.find((i) => i.id === updated.id);
		items = items?.map((i) => (i.id === updated.id ? updated : i)) ?? null;
		if (quickViewItem?.id === updated.id) quickViewItem = updated;
		if (!previous || !sameTagSet(previous.tags, updated.tags)) {
			refreshTagsInProject();
		}
	}

	function handleItemDeleted(deleted: Item) {
		items = items?.filter((i) => i.id !== deleted.id) ?? null;
		if (quickViewItem?.id === deleted.id) {
			quickViewOpen = false;
			quickViewItem = null;
		}
		if (deleted.tags.length > 0) refreshTagsInProject();
	}

	function handleItemCreated(item: Item) {
		items = [item, ...(items ?? [])];
		if (item.tags.length > 0) refreshTagsInProject();
	}

	function handleDragStart(e: DragEvent, item: Item) {
		if (!e.dataTransfer) return;
		e.dataTransfer.setData('text/plain', item.id);
		e.dataTransfer.effectAllowed = 'move';
		draggingId = item.id;
	}

	function handleDragEnd() {
		draggingId = null;
		dragOverStatus = null;
	}

	function handleDragOver(e: DragEvent, status: ItemStatus) {
		e.preventDefault(); // required to allow drop
		if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
		dragOverStatus = status;
	}

	async function handleDrop(e: DragEvent, status: ItemStatus) {
		e.preventDefault();
		dragOverStatus = null;
		const id = e.dataTransfer?.getData('text/plain');
		const dragged = items?.find((i) => i.id === id);
		if (!dragged || !project || dragged.status === status) return;

		// New position = above the current top of the target shelf.
		const targetTop = items!
			.filter((i) => i.status === status)
			.reduce((max, i) => Math.max(max, i.position), 0);
		const newPosition = targetTop + 1000;

		const snapshot = items!;
		// Optimistic update
		items = items!.map((i) => (i.id === dragged.id ? { ...i, status, position: newPosition } : i));
		// If destination is collapsed, expand so the user sees the result.
		if (status === 'done') showDone = true;
		if (status === 'archived') showArchived = true;

		try {
			const updated = await updateItem(project.slug, dragged.sequence, {
				status,
				position: newPosition
			});
			items = items!.map((i) => (i.id === updated.id ? updated : i));
		} catch (e) {
			items = snapshot;
			itemError = errMsg(e);
		}
	}

	$effect(() => {
		if (!auth.loading && !auth.user) goto('/login');
	});

	$effect(() => {
		const slug = page.params.slug;
		if (auth.user && slug && project === null) {
			getProject(slug)
				.then((res) => {
					project = res;
					return listTagsForProject(res.slug);
				})
				.then((tgs) => {
					tagsInProject = tgs;
				})
				.catch((e) => {
					loadError = errMsg(e);
				});
		}
	});

	// Items load: fires on initial project load and whenever the tag filter
	// changes. Server-side filter is the initial snapshot; client-side filter
	// in `filtered` handles items that SSE adds outside the active filter scope
	// (e.g. a teammate retags an item out of our view).
	$effect(() => {
		if (!project) return;
		const opts = selectedTagSlugs.length > 0 ? { tags: selectedTagSlugs, tagMode } : undefined;
		listItems(project.slug, opts)
			.then((res) => {
				items = res;
				itemsLastSeen = {};
			})
			.catch((e) => (itemError = errMsg(e)));
	});

	async function reloadQuickViewItem(): Promise<void> {
		if (!quickViewItem || !project) return;
		try {
			const fresh = await getItem(project.slug, quickViewItem.sequence);
			quickViewItem = fresh;
			items = items?.map((i) => (i.id === fresh.id ? fresh : i)) ?? null;
			// Drop cursors for this item so a reordered older event arriving
			// post-reload can't clobber the freshly fetched state.
			itemsLastSeen = { ...itemsLastSeen, [fresh.id]: '' };
			quickViewLastSeen = '';
			quickViewAffordance = 'none';
		} catch (e) {
			// 404 → the item really is gone; upgrade to deleted-elsewhere so the
			// "Reload" banner doesn't stay stuck behind a generic error.
			if (e instanceof ApiError && e.status === 404) {
				quickViewAffordance = 'deleted-elsewhere';
			} else {
				itemError = errMsg(e);
			}
		}
	}

	// Live updates: items list (board) + project metadata + QuickView item.
	// Listeners bind once on mount and read `project` / `items` / `quickView*`
	// lazily — keeping the effect body free of reactive reads prevents the
	// listeners from being torn down + re-bound on every applied event.
	$effect(() => {
		const unsubChanged = realtime.on('item.changed', (ev) => {
			if (!project || ev.item.project_id !== project.id) return;
			const result = applyItemEvent({ items: items ?? [], lastSeen: itemsLastSeen }, ev, null);
			items = result.items;
			itemsLastSeen = result.lastSeen;
			// Route the QuickView path through applyEditorEvent so staleness +
			// editor-isolation policy match the single-item surfaces exactly.
			if (quickViewItem && quickViewItem.id === ev.item.id) {
				const r = applyEditorEvent(
					quickViewItem,
					quickViewLastSeen,
					ev,
					quickViewEditing,
					realtime.isOwnEvent(ev.cursor)
				);
				quickViewItem = r.item;
				quickViewLastSeen = r.lastSeen;
				if (r.affordance !== 'none') quickViewAffordance = r.affordance;
			}
		});

		const unsubDeleted = realtime.on('item.deleted', (ev) => {
			if (!project || ev.payload.project_id !== project.id) return;
			const result = applyItemEvent({ items: items ?? [], lastSeen: itemsLastSeen }, ev, null);
			items = result.items;
			itemsLastSeen = result.lastSeen;
			if (quickViewItem && quickViewItem.id === ev.payload.id) {
				const r = applyEditorEvent(
					quickViewItem,
					quickViewLastSeen,
					ev,
					quickViewEditing,
					realtime.isOwnEvent(ev.cursor)
				);
				quickViewLastSeen = r.lastSeen;
				if (r.affordance !== 'none') quickViewAffordance = r.affordance;
			}
		});

		const unsubProjectChanged = realtime.on('project.changed', (ev) => {
			if (!project || ev.project.id !== project.id) return;
			const result = applyProjectEvent({ project, lastSeen: projectLastSeen }, ev);
			project = result.project;
			projectLastSeen = result.lastSeen;
		});

		const unsubResync = realtime.on('resync', () => {
			if (!project) return;
			const opts = selectedTagSlugs.length > 0 ? { tags: selectedTagSlugs, tagMode } : undefined;
			listItems(project.slug, opts)
				.then((res) => {
					items = res;
					itemsLastSeen = {};
				})
				.catch((e) => console.error('[realtime] resync listItems failed', e));
			getProject(project.slug)
				.then((p) => {
					project = p;
					projectLastSeen = '';
				})
				.catch((e) => console.error('[realtime] resync getProject failed', e));
		});

		return () => {
			unsubChanged();
			unsubDeleted();
			unsubProjectChanged();
			unsubResync();
		};
	});

	function toggleTagFilter(slug: string) {
		if (selectedTagSlugs.includes(slug)) {
			selectedTagSlugs = selectedTagSlugs.filter((s) => s !== slug);
		} else {
			selectedTagSlugs = [...selectedTagSlugs, slug];
		}
	}

	async function handleDetachTag(item: Item, tagSlug: string) {
		if (!project) return;
		const snapshot = items;
		// Optimistic remove
		items =
			items?.map((i) =>
				i.id === item.id ? { ...i, tags: i.tags.filter((t) => t.slug !== tagSlug) } : i
			) ?? null;
		try {
			await detachTagFromItem(project.slug, item.sequence, tagSlug);
			refreshTagsInProject();
		} catch (e) {
			items = snapshot;
			itemError = errMsg(e);
		}
	}

	// Client-side filter — server-side filter on listItems is the initial
	// snapshot, but SSE can add items that fall outside the active scope
	// (other actors retagging, etc.). Re-checking here keeps the visible view
	// consistent without round-tripping on every event.
	const filtered = $derived(
		items?.filter((i) => {
			if (kindFilter && i.kind !== kindFilter) return false;
			if (selectedTagSlugs.length > 0) {
				const matches =
					tagMode === 'and'
						? selectedTagSlugs.every((s) => i.tags.some((t) => t.slug === s))
						: selectedTagSlugs.some((s) => i.tags.some((t) => t.slug === s));
				if (!matches) return false;
			}
			return true;
		}) ?? []
	);

	function bucket(status: ItemStatus): Item[] {
		return filtered.filter((i) => i.status === status);
	}

	// Items in the order they appear on screen — used by the quick view panel
	// to navigate prev/next through the currently filtered list.
	const orderedItems = $derived([
		...bucket('open'),
		...bucket('in_progress'),
		...bucket('done'),
		...bucket('archived')
	]);
	const quickViewIndex = $derived(
		quickViewItem ? orderedItems.findIndex((i) => i.id === quickViewItem!.id) : -1
	);
	const prevItem = $derived(quickViewIndex > 0 ? orderedItems[quickViewIndex - 1] : null);
	const nextItem = $derived(
		quickViewIndex >= 0 && quickViewIndex < orderedItems.length - 1
			? orderedItems[quickViewIndex + 1]
			: null
	);

	function goPrev() {
		if (prevItem) {
			quickViewItem = prevItem;
			quickViewAffordance = 'none';
			quickViewLastSeen = '';
		}
	}
	function goNext() {
		if (nextItem) {
			quickViewItem = nextItem;
			quickViewAffordance = 'none';
			quickViewLastSeen = '';
		}
	}

	async function performDelete() {
		if (!project) return;
		deleting = true;
		try {
			await deleteProject(project.slug);
			goto('/');
		} catch (e) {
			loadError = errMsg(e);
			deleting = false;
		}
	}
</script>

{#snippet itemRow(item: Item)}
	<li
		draggable="true"
		ondragstart={(e) => handleDragStart(e, item)}
		ondragend={handleDragEnd}
		class="group flex cursor-grab flex-col gap-1.5 px-4 py-2.5 transition hover:bg-card-2 active:cursor-grabbing"
		class:opacity-40={draggingId === item.id}
	>
		{#if item.tags.length > 0}
			<div class="flex flex-wrap items-center gap-1 pl-7">
				{#each item.tags as tag (tag.id)}
					<ItemTagChip {tag} onRemove={() => handleDetachTag(item, tag.slug)} />
				{/each}
			</div>
		{/if}
		<div class="flex items-center gap-3">
			<StatusIcon status={item.status} />
			<button
				type="button"
				onclick={() => openQuickView(item)}
				class="min-w-0 flex-1 truncate text-left text-sm text-fg hover:underline"
			>
				{item.title}
			</button>
			<span class="rounded-full px-2 py-0.5 text-xs font-medium" style={kindChipStyle(item.kind)}>
				{KIND_LABEL[item.kind]}
			</span>
			{#if item.attachments.length > 0}
				<span
					class="inline-flex items-center gap-0.5 text-xs text-fg-faint"
					title={`${item.attachments.length} attachment${item.attachments.length === 1 ? '' : 's'}`}
				>
					<Paperclip class="h-3 w-3" />
					{item.attachments.length}
				</span>
			{/if}
			<a
				href={`/projects/${project!.slug}/${item.sequence}`}
				class="font-mono text-xs text-fg-faint hover:text-fg hover:underline"
				title="Open full view"
			>
				#{item.sequence}
			</a>
			<span
				class="hidden w-10 text-right text-xs text-fg-faint sm:inline"
				title={new Date(item.updated_at).toLocaleString()}
			>
				{relativeTime(item.updated_at)}
			</span>
		</div>
	</li>
{/snippet}

{#snippet shelf(status: ItemStatus, defaultOpen: boolean)}
	{@const list = bucket(status)}
	{@const expanded = defaultOpen}
	{@const isDropTarget = dragOverStatus === status && draggingId !== null}
	<section
		aria-label={`${STATUS_LABEL[status]} items`}
		ondragover={(e) => handleDragOver(e, status)}
		ondragleave={() => (dragOverStatus = null)}
		ondrop={(e) => handleDrop(e, status)}
		class="overflow-hidden rounded-lg border bg-card transition"
		class:border-line={!isDropTarget}
		class:border-accent={isDropTarget}
		class:ring-2={isDropTarget}
		class:ring-accent={isDropTarget}
		class:ring-offset-2={isDropTarget}
		class:ring-offset-app={isDropTarget}
		style:border-top-color={isDropTarget
			? 'var(--color-accent)'
			: projectColourVar(project!.colour)}
		style:border-top-width="2px"
	>
		<header class="flex items-center justify-between border-b border-line bg-card-2/40 px-4 py-2">
			<h3 class="text-sm font-medium text-fg">{STATUS_LABEL[status]}</h3>
			<span class="text-xs text-fg-faint">{list.length}</span>
		</header>
		{#if expanded}
			{#if list.length === 0}
				<p class="px-4 py-6 text-center text-xs text-fg-faint">
					{isDropTarget ? 'Drop to move here.' : 'Nothing here yet.'}
				</p>
			{:else}
				<ul class="divide-y divide-line/40">
					{#each list as item (item.id)}
						{@render itemRow(item)}
					{/each}
				</ul>
			{/if}
		{/if}
	</section>
{/snippet}

{#if !auth.loading && auth.user}
	<main class="mx-auto max-w-6xl px-6 py-10">
		<div class="mb-6">
			<a
				href="/"
				class="inline-flex items-center gap-1.5 text-sm text-fg-muted hover:text-fg hover:underline"
			>
				<ArrowLeft class="h-4 w-4" />
				All projects
			</a>
		</div>

		{#if loadError}
			<div class="rounded-md border border-danger/40 bg-danger/10 p-4">
				<p class="text-sm text-danger">{loadError}</p>
			</div>
		{:else if project === null}
			<p class="text-sm text-fg-faint">Loading…</p>
		{:else}
			<header
				class="mb-8 border-b-4 pb-6"
				style:border-bottom-color={projectColourVar(project.colour)}
			>
				<div class="flex items-start gap-3 sm:gap-4">
					{#if project.icon}
						<span class="text-3xl leading-none sm:text-4xl">{project.icon}</span>
					{/if}
					<div class="min-w-0 flex-1">
						<div class="flex items-start justify-between gap-2">
							<h1 class="text-2xl font-semibold tracking-tight text-fg sm:text-3xl">
								{project.name}
							</h1>
							<div class="flex shrink-0 items-center gap-1">
								<button
									type="button"
									onclick={() => (activityPanelOpen = true)}
									aria-label="Project activity"
									title="Activity"
									class="rounded-md p-2 text-fg-muted transition hover:bg-card-2 hover:text-fg"
								>
									<Activity class="h-4 w-4" />
								</button>
								<a
									href={`/projects/${project.slug}/edit`}
									aria-label="Edit project"
									title="Edit"
									class="rounded-md p-2 text-fg-muted transition hover:bg-card-2 hover:text-fg"
								>
									<Pencil class="h-4 w-4" />
								</a>
								<button
									type="button"
									onclick={() => (deleteConfirmOpen = true)}
									disabled={deleting}
									aria-label="Delete project"
									title="Delete"
									class="rounded-md p-2 text-fg-muted transition hover:bg-danger/10 hover:text-danger disabled:opacity-50"
								>
									<Trash2 class="h-4 w-4" />
								</button>
							</div>
						</div>
						{#if project.description}
							<p class="mt-2 text-fg-muted">{project.description}</p>
						{/if}
					</div>
				</div>
			</header>

			{#if items === null}
				<p class="text-sm text-fg-faint">Loading items…</p>
			{:else}
				<!-- Filter + new -->
				<div class="mb-5 flex flex-wrap items-center gap-2">
					<button
						type="button"
						onclick={() => (kindFilter = null)}
						class="rounded-full px-3 py-1 text-xs font-medium transition"
						class:bg-fg={kindFilter === null}
						class:text-on-accent={kindFilter === null}
						class:text-fg-muted={kindFilter !== null}
						class:hover:bg-card-2={kindFilter !== null}
					>
						All
					</button>
					{#each ITEM_KINDS as k (k)}
						<button
							type="button"
							onclick={() => (kindFilter = k)}
							class="rounded-full px-3 py-1 text-xs font-medium transition"
							class:bg-fg={kindFilter === k}
							class:text-on-accent={kindFilter === k}
							class:text-fg-muted={kindFilter !== k}
							class:hover:bg-card-2={kindFilter !== k}
						>
							{KIND_PLURAL[k]}
						</button>
					{/each}
					{#if tagsInProject.length > 0}
						<span class="mx-1 h-4 w-px bg-line"></span>
						<TagFilterPopover
							tags={tagsInProject}
							selectedSlugs={selectedTagSlugs}
							mode={tagMode}
							onToggle={toggleTagFilter}
							onModeChange={(m) => (tagMode = m)}
						/>
					{/if}
					<span class="flex-1"></span>
					<button
						type="button"
						onclick={() => (addItemOpen = true)}
						class="inline-flex items-center gap-1.5 rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-on-accent transition hover:bg-accent-hover"
					>
						<Plus class="h-4 w-4" />
						New item
					</button>
				</div>

				{#if itemError}
					<div
						class="mb-4 flex items-start gap-3 rounded-md border border-danger/40 bg-danger/10 px-3 py-2 text-sm text-danger"
					>
						<span class="flex-1">{itemError}</span>
						<button
							type="button"
							onclick={() => (itemError = null)}
							aria-label="Dismiss"
							class="text-danger/70 transition hover:text-danger"
						>
							<X class="h-4 w-4" />
						</button>
					</div>
				{/if}

				{#if items.length === 0}
					<section class="rounded-lg border border-line bg-card p-6 text-center sm:p-12">
						<p class="text-fg-muted">Your trove is empty — toss something in.</p>
					</section>
				{:else}
					<div class="flex flex-col gap-4">
						{@render shelf('open', true)}
						{@render shelf('in_progress', true)}

						<div class="flex flex-wrap items-center gap-2 pt-2">
							<button
								type="button"
								onclick={() => (showDone = !showDone)}
								ondragover={(e) => handleDragOver(e, 'done')}
								ondragleave={() => (dragOverStatus = null)}
								ondrop={(e) => handleDrop(e, 'done')}
								class="inline-flex items-center gap-1 rounded-md border border-transparent px-2 py-1 text-xs font-medium text-fg-muted transition hover:bg-card-2 hover:text-fg"
								class:border-accent={dragOverStatus === 'done' && draggingId !== null}
							>
								{#if showDone}
									<ChevronDown class="h-3.5 w-3.5" />
								{:else}
									<ChevronRight class="h-3.5 w-3.5" />
								{/if}
								{dragOverStatus === 'done' && draggingId !== null
									? 'Drop to mark done'
									: `Done · ${bucket('done').length}`}
							</button>
							<button
								type="button"
								onclick={() => (showArchived = !showArchived)}
								ondragover={(e) => handleDragOver(e, 'archived')}
								ondragleave={() => (dragOverStatus = null)}
								ondrop={(e) => handleDrop(e, 'archived')}
								class="inline-flex items-center gap-1 rounded-md border border-transparent px-2 py-1 text-xs font-medium text-fg-muted transition hover:bg-card-2 hover:text-fg"
								class:border-accent={dragOverStatus === 'archived' && draggingId !== null}
							>
								{#if showArchived}
									<ChevronDown class="h-3.5 w-3.5" />
								{:else}
									<ChevronRight class="h-3.5 w-3.5" />
								{/if}
								{dragOverStatus === 'archived' && draggingId !== null
									? 'Drop to archive'
									: `Archived · ${bucket('archived').length}`}
							</button>
						</div>

						{#if showDone}
							{@render shelf('done', true)}
						{/if}
						{#if showArchived}
							{@render shelf('archived', true)}
						{/if}
					</div>
				{/if}
			{/if}

			<div class="mt-6">
				<ActivityStrip {project} onOpenPanel={() => (activityPanelOpen = true)} />
			</div>

			<ConfirmDialog
				bind:open={deleteConfirmOpen}
				title={`Delete "${project.name}"?`}
				message="This is permanent. The project and all of its items, tags, attachments, and activity will be removed."
				confirmLabel="Delete project"
				cancelLabel="Keep it"
				destructive={true}
				onConfirm={performDelete}
			/>

			<AddItemDialog
				bind:open={addItemOpen}
				projectSlug={project.slug}
				onCreated={handleItemCreated}
			/>

			<QuickViewPanel
				bind:open={quickViewOpen}
				bind:editing={quickViewEditing}
				item={quickViewItem}
				{project}
				affordance={quickViewAffordance}
				onReload={reloadQuickViewItem}
				onUpdated={handleItemUpdated}
				onDeleted={handleItemDeleted}
				onPrev={prevItem ? goPrev : undefined}
				onNext={nextItem ? goNext : undefined}
			/>

			<ActivityPanel bind:open={activityPanelOpen} {project} />
		{/if}
	</main>
{/if}
