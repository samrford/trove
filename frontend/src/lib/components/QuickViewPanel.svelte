<script lang="ts">
	import {
		updateItem,
		deleteItem,
		getItem,
		attachTagToItem,
		detachTagFromItem,
		type Item,
		type Attachment,
		type ItemKind,
		type ItemStatus,
		ITEM_KINDS,
		ITEM_STATUSES
	} from '$lib/api/items';
	import { listTags, type Tag, type TagWithCount } from '$lib/api/tags';
	import type { Project } from '$lib/api/projects';
	import { errMsg } from '$lib/api';
	import type { EditorAffordance } from '$lib/realtime';
	import { animations } from '$lib/animations.svelte';
	import { appConfig } from '$lib/config.svelte';
	import { KIND_LABEL, STATUS_LABEL, kindChipStyle } from '$lib/itemDisplay';
	import { tagColourFromName } from '$lib/tagColours';
	import ConfirmDialog from './ConfirmDialog.svelte';
	import StatusIcon from './StatusIcon.svelte';
	import ItemBody from './ItemBody.svelte';
	import TagChip from './TagChip.svelte';
	import TagCombobox from './TagCombobox.svelte';
	import AttachmentList from './AttachmentList.svelte';
	import AttachmentUploader from './AttachmentUploader.svelte';
	import GooglePhotosImportFlow from './GooglePhotosImportFlow.svelte';
	import ActivityRail from './ActivityRail.svelte';
	import { fly } from 'svelte/transition';
	import { backInOut } from 'svelte/easing';
	import {
		X,
		ExternalLink,
		Trash2,
		Pencil,
		Check,
		ChevronLeft,
		ChevronRight
	} from '@lucide/svelte';

	type Props = {
		open?: boolean;
		// Bindable so the parent knows the editor is open (e.g. to reset on item
		// switch).
		editing?: boolean;
		// Bindable real-dirtiness (scratch differs from the seeded baseline) so
		// the parent's SSE listener can silently sync the item prop when clean, or
		// raise an affordance only when there are genuine unsaved edits.
		dirty?: boolean;
		item: Item | null;
		project: Project | null;
		// Parent-managed: set by the parent's realtime listener when a remote
		// change targets this item while editing, or when it's deleted.
		affordance?: EditorAffordance;
		// Parent-provided refetch — clears the affordance + re-syncs the prop.
		onReload?: () => void | Promise<void>;
		onUpdated?: (item: Item) => void;
		onDeleted?: (item: Item) => void;
		onPrev?: () => void;
		onNext?: () => void;
	};

	let {
		open = $bindable(false),
		editing = $bindable(false),
		// $bindable() fallback — overwritten by the `dirty = isDirty` $effect below.
		// eslint-disable-next-line no-useless-assignment
		dirty = $bindable(false),
		item,
		project,
		affordance = 'none',
		onReload,
		onUpdated,
		onDeleted,
		onPrev,
		onNext
	}: Props = $props();

	let actionError = $state<string | null>(null);
	let deleteConfirmOpen = $state(false);
	let saving = $state(false);
	let editTitle = $state('');
	let editBody = $state('');
	let editKind = $state<ItemKind>('task');
	// Baseline the scratch was seeded from. Dirtiness is measured against this
	// (not the live item) so a server-sync under a clean editor isn't mistaken
	// for a local edit. Status/tags are applied immediately, so they're not part
	// of the editor scratch.
	let seedBase = $state<{ title: string; body: string; kind: ItemKind } | null>(null);
	const isDirty = $derived(
		editing &&
			!!seedBase &&
			(editTitle !== seedBase.title || editBody !== seedBase.body || editKind !== seedBase.kind)
	);
	// Surface real-dirtiness to the parent (drives the editor-isolation policy).
	$effect(() => {
		dirty = isDirty;
	});

	let availableTags = $state<TagWithCount[]>([]);

	let gphotosOpen = $state(false);
	const attachmentsEnabled = $derived(appConfig.config?.attachmentsEnabled ?? false);

	// After an attachment changes, re-fetch the item so the parent (and us)
	// see fresh signed URLs and a fresh attachments list.
	async function refreshItemFromServer() {
		if (!item || !project) return;
		try {
			const fresh = await getItem(project.slug, item.sequence);
			onUpdated?.(fresh);
		} catch (e) {
			actionError = errMsg(e);
		}
	}

	function handleAttachmentUploaded(_a: Attachment) {
		refreshItemFromServer();
	}
	function handleAttachmentDeleted(_a: Attachment) {
		refreshItemFromServer();
	}

	async function refreshAvailableTags() {
		try {
			availableTags = await listTags();
		} catch {
			// Tag list is non-critical for edit; ignore.
		}
	}

	async function handleAddTag(
		t: Tag | { name: string; colour?: string | null; icon?: string | null }
	) {
		if (!item || !project) return;
		try {
			const tag =
				'id' in t
					? await attachTagToItem(project.slug, item.sequence, { tag_id: t.id })
					: await attachTagToItem(project.slug, item.sequence, {
							name: t.name,
							colour: t.colour ?? tagColourFromName(t.name),
							icon: t.icon ?? null
						});
			const updated = { ...item, tags: [...item.tags, tag] };
			onUpdated?.(updated);
		} catch (e) {
			actionError = errMsg(e);
		}
	}

	async function handleRemoveTag(t: Tag) {
		if (!item || !project) return;
		try {
			await detachTagFromItem(project.slug, item.sequence, t.slug);
			const updated = { ...item, tags: item.tags.filter((x) => x.id !== t.id) };
			onUpdated?.(updated);
		} catch (e) {
			actionError = errMsg(e);
		}
	}

	function close() {
		if (editing) return; // don't lose unsaved edits via backdrop / X
		open = false;
		actionError = null;
	}

	function seedEditScratch() {
		if (!item) return;
		editTitle = item.title;
		editBody = item.body ?? '';
		editKind = item.kind;
		seedBase = { title: item.title, body: item.body ?? '', kind: item.kind };
	}

	function startEdit() {
		if (!item) return;
		seedEditScratch();
		actionError = null;
		editing = true;
		refreshAvailableTags();
	}

	// Re-seed the editor when the item is server-synced underneath a CLEAN (no
	// local edits) open editor, so the form tracks the new base rather than
	// silently saving over a remote change. Dirty editors raise an affordance
	// (via the parent) and are left untouched.
	$effect(() => {
		if (!editing || !item || !seedBase || isDirty) return;
		if (
			item.title !== seedBase.title ||
			(item.body ?? '') !== seedBase.body ||
			item.kind !== seedBase.kind
		) {
			seedEditScratch();
		}
	});

	function cancelEdit() {
		editing = false;
		actionError = null;
	}

	// Reload affordance handler: parent refetches the item; we drop edit
	// mode so the freshly synced prop is what the user sees.
	async function handleReload() {
		await onReload?.();
		cancelEdit();
	}

	async function saveEdit() {
		if (!item || !project) return;
		const trimmed = editTitle.trim();
		if (!trimmed) {
			actionError = 'Title is required';
			return;
		}
		saving = true;
		actionError = null;
		try {
			const updated = await updateItem(project.slug, item.sequence, {
				title: trimmed,
				body: editBody, // empty string clears on server
				kind: editKind
			});
			onUpdated?.(updated);
			editing = false;
		} catch (e) {
			actionError = errMsg(e);
		} finally {
			saving = false;
		}
	}

	// Width of the panel + small gap. Pushes the page content to the left so
	// the panel sits alongside it instead of covering it. Tailwind `max-w-md` = 28rem.
	const PANEL_OFFSET = '28rem';
	// Below this breakpoint the panel becomes a full-width overlay instead of a
	// side-by-side panel, so we skip the body offset.
	const SIDE_BY_SIDE_QUERY = '(min-width: 640px)';

	$effect(() => {
		if (!open) return;

		const prevPadding = document.body.style.paddingRight;
		const prevTransition = document.body.style.transition;
		document.body.style.transition = 'padding-right 200ms ease';

		const mq = window.matchMedia(SIDE_BY_SIDE_QUERY);
		function applyOffset() {
			document.body.style.paddingRight = mq.matches ? PANEL_OFFSET : '';
		}
		applyOffset();
		mq.addEventListener('change', applyOffset);

		function onKey(e: KeyboardEvent) {
			if (e.key === 'Escape') {
				if (editing) cancelEdit();
				else {
					open = false;
					actionError = null;
				}
				return;
			}
			if (editing) return;
			// Don't hijack arrow keys when the user is typing or interacting with a control.
			const target = e.target as HTMLElement | null;
			if (
				target &&
				(target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)
			) {
				return;
			}
			if (e.key === 'ArrowLeft' && onPrev) {
				e.preventDefault();
				onPrev();
			} else if (e.key === 'ArrowRight' && onNext) {
				e.preventDefault();
				onNext();
			}
		}
		document.addEventListener('keydown', onKey);

		return () => {
			document.removeEventListener('keydown', onKey);
			mq.removeEventListener('change', applyOffset);
			document.body.style.paddingRight = prevPadding;
			// Clear the transition shortly after so it doesn't linger on unrelated changes.
			setTimeout(() => {
				document.body.style.transition = prevTransition;
			}, 220);
		};
	});

	async function setStatus(status: ItemStatus) {
		if (!item || !project || item.status === status) return;
		actionError = null;
		try {
			const updated = await updateItem(project.slug, item.sequence, { status });
			onUpdated?.(updated);
		} catch (e) {
			actionError = errMsg(e);
		}
	}

	async function performDelete() {
		if (!item || !project) return;
		try {
			await deleteItem(project.slug, item.sequence);
			onDeleted?.(item);
			open = false;
		} catch (e) {
			actionError = errMsg(e);
		}
	}

	// Page-turn defaults locked in from the demo. The transition no-ops when
	// the user has disabled animations (or has OS prefers-reduced-motion set
	// with no override).
	const FLIP_DURATION = 1800;
	const FLIP_PERSPECTIVE = 2500;

	function pageFlipOut(_node: HTMLElement) {
		if (!animations.enabled) return { duration: 0, css: () => '' };
		return {
			duration: FLIP_DURATION,
			easing: backInOut,
			css: (t: number) => `
				transform: perspective(${FLIP_PERSPECTIVE}px) rotateY(${-180 + 180 * t}deg);
				transform-origin: center;
				backface-visibility: hidden;
				box-shadow: 0 ${(1 - t) * 30}px ${(1 - t) * 60}px rgba(0,0,0,${0.4 * (1 - t)});
			`
		};
	}

	function pageFlipIn(_node: HTMLElement) {
		if (!animations.enabled) return { duration: 0, css: () => '' };
		return {
			duration: FLIP_DURATION,
			easing: backInOut,
			css: (t: number) => `
				transform: perspective(${FLIP_PERSPECTIVE}px) rotateY(${180 * (1 - t)}deg);
				transform-origin: center;
				backface-visibility: hidden;
				box-shadow: 0 ${t * 30}px ${t * 60}px rgba(0,0,0,${0.4 * t});
			`
		};
	}
</script>

{#if open && item && project}
	<!-- Mobile backdrop (panel becomes an overlay on small screens) -->
	<button
		type="button"
		aria-label="Close"
		onclick={close}
		class="fixed inset-0 z-30 bg-black/40 backdrop-blur-sm sm:hidden"
	></button>

	<aside
		transition:fly={{ x: 400, duration: 200 }}
		class="fixed inset-y-0 right-0 z-40 flex w-full max-w-md flex-col border-l border-line bg-card shadow-2xl"
	>
		<!-- Header -->
		<header class="flex items-center justify-between border-b border-line px-4 py-3">
			<div class="flex flex-wrap items-center gap-2">
				<span class="rounded-full px-2 py-0.5 text-xs font-medium" style={kindChipStyle(item.kind)}>
					{KIND_LABEL[item.kind]}
				</span>
				<span class="font-mono text-xs text-fg-faint">#{item.sequence}</span>
			</div>
			<div class="flex items-center gap-1">
				<button
					type="button"
					onclick={() => onPrev?.()}
					disabled={!onPrev || editing}
					aria-label="Previous item"
					title="Previous item"
					class="rounded-md p-1.5 text-fg-muted transition hover:bg-card-2 hover:text-fg disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-fg-muted"
				>
					<ChevronLeft class="h-4 w-4" />
				</button>
				<button
					type="button"
					onclick={() => onNext?.()}
					disabled={!onNext || editing}
					aria-label="Next item"
					title="Next item"
					class="rounded-md p-1.5 text-fg-muted transition hover:bg-card-2 hover:text-fg disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-fg-muted"
				>
					<ChevronRight class="h-4 w-4" />
				</button>
				{#if !editing}
					<button
						type="button"
						onclick={startEdit}
						aria-label="Edit"
						title="Edit"
						class="rounded-md p-1.5 text-fg-muted transition hover:bg-card-2 hover:text-fg"
					>
						<Pencil class="h-4 w-4" />
					</button>
				{/if}
				<button
					type="button"
					onclick={close}
					disabled={editing}
					aria-label="Close"
					title={editing ? 'Cancel or save first' : 'Close'}
					class="rounded-md p-1.5 text-fg-muted transition hover:bg-card-2 hover:text-fg disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent"
				>
					<X class="h-4 w-4" />
				</button>
			</div>
		</header>

		<!-- Editor-isolation affordance — non-destructive banner so a remote
			 change to this item doesn't destroy an open editor. -->
		{#if affordance === 'updated-elsewhere'}
			<div
				class="flex items-center gap-2 border-b border-accent/40 bg-accent/10 px-4 py-2 text-xs text-accent"
				role="status"
			>
				<span class="flex-1">This item was changed elsewhere.</span>
				<button
					type="button"
					onclick={handleReload}
					class="rounded-md border border-accent/40 px-2 py-0.5 font-medium text-accent transition hover:bg-accent/20"
				>
					Reload
				</button>
			</div>
		{:else if affordance === 'deleted-elsewhere'}
			<div
				class="flex items-center gap-2 border-b border-danger/40 bg-danger/10 px-4 py-2 text-xs text-danger"
				role="status"
			>
				<span class="flex-1">This item was deleted elsewhere.</span>
				<button
					type="button"
					onclick={() => {
						if (editing) cancelEdit();
						open = false;
					}}
					class="rounded-md border border-danger/40 px-2 py-0.5 font-medium text-danger transition hover:bg-danger/20"
				>
					Close
				</button>
			</div>
		{/if}

		<!-- Scrollable content -->
		{#if editing}
			<div class="flex-1 overflow-y-auto px-5 py-4">
				<div class="flex flex-col gap-4">
					<label class="flex flex-col gap-1.5">
						<span class="text-xs font-medium tracking-wide text-fg-faint uppercase"> Title </span>
						<input
							type="text"
							bind:value={editTitle}
							required
							maxlength={200}
							class="rounded-md border border-line bg-card px-3 py-2 text-base font-semibold text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
						/>
					</label>

					<div class="flex flex-col gap-1.5">
						<span class="text-xs font-medium tracking-wide text-fg-faint uppercase">Kind</span>
						<div class="flex flex-wrap gap-1.5">
							{#each ITEM_KINDS as k (k)}
								<button
									type="button"
									onclick={() => (editKind = k)}
									style={kindChipStyle(k, editKind === k)}
									class="rounded-full border-2 px-3 py-0.5 text-xs font-medium transition"
									class:border-transparent={editKind !== k}
								>
									{KIND_LABEL[k]}
								</button>
							{/each}
						</div>
					</div>

					<label class="flex flex-col gap-1.5">
						<span class="text-xs font-medium tracking-wide text-fg-faint uppercase">
							Notes <span class="font-normal text-fg-faint normal-case">(markdown)</span>
						</span>
						<textarea
							bind:value={editBody}
							rows="10"
							placeholder="Anything more to say?"
							class="resize-y rounded-md border border-line bg-card px-3 py-2 font-mono text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
						></textarea>
					</label>

					<div class="flex flex-col gap-1.5">
						<span class="text-xs font-medium tracking-wide text-fg-faint uppercase">Tags</span>
						<TagCombobox
							selected={item.tags}
							{availableTags}
							onAdd={handleAddTag}
							onRemove={handleRemoveTag}
							placeholder="Add tags…"
						/>
					</div>

					{#if attachmentsEnabled}
						<div class="flex flex-col gap-1.5">
							<span class="text-xs font-medium tracking-wide text-fg-faint uppercase"
								>Attachments</span
							>
							<AttachmentUploader
								slug={project.slug}
								seq={item.sequence}
								onUploaded={handleAttachmentUploaded}
								onGooglePhotosClick={() => (gphotosOpen = true)}
							/>
							<AttachmentList
								slug={project.slug}
								seq={item.sequence}
								attachments={item.attachments}
								onDeleted={handleAttachmentDeleted}
							/>
						</div>
					{/if}
				</div>
			</div>
		{:else}
			<!-- Read mode: inner card flips on item change. Card sizes to its content
				 but is capped at the available panel height (scrolls if longer). -->
			<div
				class="relative flex-1 overflow-hidden px-4 py-4"
				style="perspective: {FLIP_PERSPECTIVE}px;"
			>
				{#key item.id}
					<div
						in:pageFlipIn
						out:pageFlipOut
						class="absolute top-4 right-4 left-4 max-h-[calc(100%-2rem)] overflow-y-auto rounded-lg border border-line bg-card-2 p-5"
					>
						<h2 class="text-xl font-semibold tracking-tight text-fg">{item.title}</h2>
						{#if item.tags.length > 0}
							<div class="mt-3 flex flex-wrap gap-1.5">
								{#each item.tags as tag (tag.id)}
									<TagChip {tag} size="sm" />
								{/each}
							</div>
						{/if}
						<ItemBody {item} class="mt-4" />
						{#if attachmentsEnabled && item.attachments.length > 0}
							<div class="mt-5 border-t border-line pt-4">
								<h3 class="mb-2 text-xs font-medium tracking-wide text-fg-faint uppercase">
									Attachments
								</h3>
								<AttachmentList
									slug={project.slug}
									seq={item.sequence}
									attachments={item.attachments}
									onDeleted={handleAttachmentDeleted}
								/>
							</div>
						{/if}
						<div class="mt-5 border-t border-line pt-4">
							<h3 class="mb-2 text-xs font-medium tracking-wide text-fg-faint uppercase">
								Activity
							</h3>
							<ActivityRail slug={project.slug} itemId={item.id} limit={3} />
						</div>
					</div>
				{/key}
			</div>

			{#if attachmentsEnabled}
				<!-- Persistent uploader, sits below the flipping card so it stays put
					 when you navigate between items. -->
				<div class="border-t border-line px-4 py-3">
					<AttachmentUploader
						slug={project.slug}
						seq={item.sequence}
						onUploaded={handleAttachmentUploaded}
						onGooglePhotosClick={() => (gphotosOpen = true)}
					/>
				</div>
			{/if}
		{/if}

		<!-- Footer: status (always) + actions (view-mode vs edit-mode) -->
		<footer class="flex flex-col gap-3 border-t border-line px-4 py-3">
			<div class="flex flex-wrap items-center gap-1.5">
				<span class="mr-1 text-xs font-medium tracking-wide text-fg-faint uppercase"> Status </span>
				{#each ITEM_STATUSES as s (s)}
					<button
						type="button"
						onclick={() => setStatus(s)}
						disabled={editing}
						class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium transition disabled:cursor-not-allowed disabled:opacity-50"
						class:border-accent={item.status === s}
						class:bg-accent={item.status === s}
						class:text-on-accent={item.status === s}
						class:border-line={item.status !== s}
						class:text-fg-muted={item.status !== s}
						class:hover:bg-card-2={item.status !== s && !editing}
					>
						<StatusIcon status={s} class="h-3 w-3" />
						{STATUS_LABEL[s]}
					</button>
				{/each}
			</div>

			{#if actionError}
				<p class="text-xs text-danger">{actionError}</p>
			{/if}

			{#if editing}
				<div class="flex items-center justify-end gap-2">
					<button
						type="button"
						onclick={cancelEdit}
						disabled={saving}
						class="inline-flex items-center gap-1.5 rounded-md border border-line px-3 py-1.5 text-xs font-medium text-fg transition hover:bg-card-2 disabled:opacity-50"
					>
						Cancel
					</button>
					<button
						type="button"
						onclick={saveEdit}
						disabled={saving || !editTitle.trim()}
						class="inline-flex items-center gap-1.5 rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-on-accent transition hover:bg-accent-hover disabled:opacity-50"
					>
						<Check class="h-3.5 w-3.5" />
						{saving ? 'Saving…' : 'Save'}
					</button>
				</div>
			{:else}
				<div class="flex items-center justify-between gap-2">
					<a
						href={`/projects/${project.slug}/${item.sequence}`}
						class="inline-flex items-center gap-1.5 text-xs text-fg-muted hover:text-fg hover:underline"
					>
						<ExternalLink class="h-3.5 w-3.5" />
						Open full view
					</a>
					<button
						type="button"
						onclick={() => (deleteConfirmOpen = true)}
						class="inline-flex items-center gap-1.5 text-xs text-fg-muted transition hover:text-danger"
					>
						<Trash2 class="h-3.5 w-3.5" />
						Delete
					</button>
				</div>
			{/if}
		</footer>

		<ConfirmDialog
			bind:open={deleteConfirmOpen}
			title={`Delete item #${item.sequence}?`}
			message="This is permanent. The item and its attachments will be deleted. Its activity history is kept."
			confirmLabel="Delete item"
			cancelLabel="Keep it"
			destructive={true}
			onConfirm={performDelete}
		/>
	</aside>

	<GooglePhotosImportFlow
		bind:open={gphotosOpen}
		slug={project.slug}
		seq={item.sequence}
		onImported={refreshItemFromServer}
	/>
{/if}
