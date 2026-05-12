<script lang="ts">
	import {
		updateItem,
		deleteItem,
		type Item,
		type ItemKind,
		type ItemStatus,
		ITEM_KINDS,
		ITEM_STATUSES
	} from '$lib/api/items';
	import type { Project } from '$lib/api/projects';
	import { ApiError } from '$lib/api';
	import { animations } from '$lib/animations.svelte';
	import { KIND_LABEL, STATUS_LABEL, kindChipStyle } from '$lib/itemDisplay';
	import ConfirmDialog from './ConfirmDialog.svelte';
	import StatusIcon from './StatusIcon.svelte';
	import ItemBody from './ItemBody.svelte';
	import { fly } from 'svelte/transition';
	import { backInOut } from 'svelte/easing';
	import { X, ExternalLink, Trash2, Pencil, Check } from '@lucide/svelte';

	type Props = {
		open?: boolean;
		item: Item | null;
		project: Project | null;
		onUpdated?: (item: Item) => void;
		onDeleted?: (item: Item) => void;
	};

	let { open = $bindable(false), item, project, onUpdated, onDeleted }: Props = $props();

	let actionError = $state<string | null>(null);
	let deleteConfirmOpen = $state(false);

	let editing = $state(false);
	let saving = $state(false);
	let editTitle = $state('');
	let editBody = $state('');
	let editKind = $state<ItemKind>('task');

	function close() {
		if (editing) return; // don't lose unsaved edits via backdrop / X
		open = false;
		actionError = null;
	}

	function startEdit() {
		if (!item) return;
		editTitle = item.title;
		editBody = item.body ?? '';
		editKind = item.kind;
		actionError = null;
		editing = true;
	}

	function cancelEdit() {
		editing = false;
		actionError = null;
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
			actionError = e instanceof ApiError ? e.message : String(e);
		} finally {
			saving = false;
		}
	}

	// Width of the panel + small gap. Pushes the page content to the left so
	// the panel sits alongside it instead of covering it. Tailwind `max-w-md` = 28rem.
	const PANEL_OFFSET = '28rem';

	$effect(() => {
		if (!open) return;

		// Push page content left to make room for the panel.
		const prevPadding = document.body.style.paddingRight;
		const prevTransition = document.body.style.transition;
		document.body.style.transition = 'padding-right 200ms ease';
		document.body.style.paddingRight = PANEL_OFFSET;

		function onKey(e: KeyboardEvent) {
			if (e.key !== 'Escape') return;
			if (editing) cancelEdit();
			else {
				open = false;
				actionError = null;
			}
		}
		document.addEventListener('keydown', onKey);

		return () => {
			document.removeEventListener('keydown', onKey);
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
			actionError = e instanceof ApiError ? e.message : String(e);
		}
	}

	async function performDelete() {
		if (!item || !project) return;
		try {
			await deleteItem(project.slug, item.sequence);
			onDeleted?.(item);
			open = false;
		} catch (e) {
			actionError = e instanceof ApiError ? e.message : String(e);
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
				</div>
			</div>
		{:else}
			<!-- Read mode: inner card flips on item change -->
			<div class="relative flex-1 px-4 py-4" style="perspective: {FLIP_PERSPECTIVE}px;">
				{#key item.id}
					<div
						in:pageFlipIn
						out:pageFlipOut
						class="absolute inset-4 overflow-y-auto rounded-lg border border-line bg-card-2 p-5"
					>
						<h2 class="text-xl font-semibold tracking-tight text-fg">{item.title}</h2>
						<ItemBody {item} class="mt-4" />
					</div>
				{/key}
			</div>
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
			message="This is permanent. The item and its future attachments / activity will be removed."
			confirmLabel="Delete item"
			cancelLabel="Keep it"
			destructive={true}
			onConfirm={performDelete}
		/>
	</aside>
{/if}
