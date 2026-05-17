<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { getProject, type Project } from '$lib/api/projects';
	import {
		getItem,
		updateItem,
		deleteItem,
		type Item,
		type ItemKind,
		type ItemStatus
	} from '$lib/api/items';
	import { ApiError, errMsg } from '$lib/api';
	import { appConfig } from '$lib/config.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import StatusIcon from '$lib/components/StatusIcon.svelte';
	import KindPicker from '$lib/components/KindPicker.svelte';
	import StatusPicker from '$lib/components/StatusPicker.svelte';
	import ItemBody from '$lib/components/ItemBody.svelte';
	import AttachmentList from '$lib/components/AttachmentList.svelte';
	import AttachmentUploader from '$lib/components/AttachmentUploader.svelte';
	import GooglePhotosImportFlow from '$lib/components/GooglePhotosImportFlow.svelte';
	import ActivityRail from '$lib/components/ActivityRail.svelte';
	import { projectColourVar } from '$lib/projectColours';
	import { KIND_LABEL, STATUS_LABEL, kindChipStyle } from '$lib/itemDisplay';
	import { ArrowLeft, Pencil, Trash2, Check, X } from '@lucide/svelte';

	let project = $state<Project | null>(null);
	let item = $state<Item | null>(null);
	let loadError = $state<string | null>(null);
	let notFound = $state(false);
	let saveError = $state<string | null>(null);
	let editing = $state(false);
	let saving = $state(false);
	let deleteConfirmOpen = $state(false);
	let gphotosOpen = $state(false);
	const attachmentsEnabled = $derived(appConfig.config?.attachmentsEnabled ?? false);

	async function refreshItemFromServer() {
		if (!project || !item) return;
		try {
			item = await getItem(project.slug, item.sequence);
		} catch (e) {
			saveError = errMsg(e);
		}
	}

	// Edit-mode scratch state
	let editTitle = $state('');
	let editBody = $state('');
	let editKind = $state<ItemKind>('task');
	let editStatus = $state<ItemStatus>('open');

	$effect(() => {
		if (!auth.loading && !auth.user) goto('/login');
	});

	$effect(() => {
		const slug = page.params.slug;
		const seqStr = page.params.seq;
		const seq = Number(seqStr);
		if (!auth.user || !slug || !seqStr || Number.isNaN(seq) || item !== null) return;

		Promise.all([getProject(slug), getItem(slug, seq)])
			.then(([p, i]) => {
				project = p;
				item = i;
			})
			.catch((e) => {
				if (e instanceof ApiError && e.status === 404) {
					notFound = true;
				} else {
					loadError = errMsg(e);
				}
			});
	});

	function startEdit() {
		if (!item) return;
		editTitle = item.title;
		editBody = item.body ?? '';
		editKind = item.kind;
		editStatus = item.status;
		saveError = null;
		editing = true;
	}

	function cancelEdit() {
		editing = false;
		saveError = null;
	}

	async function saveEdit() {
		if (!project || !item) return;
		const trimmedTitle = editTitle.trim();
		if (!trimmedTitle) {
			saveError = 'Title is required';
			return;
		}
		saving = true;
		saveError = null;
		try {
			const updated = await updateItem(project.slug, item.sequence, {
				title: trimmedTitle,
				body: editBody, // empty string clears
				kind: editKind,
				status: editStatus
			});
			item = updated;
			editing = false;
		} catch (e) {
			saveError = errMsg(e);
		} finally {
			saving = false;
		}
	}

	async function performDelete() {
		if (!project || !item) return;
		try {
			await deleteItem(project.slug, item.sequence);
			goto(`/projects/${project.slug}`);
		} catch (e) {
			saveError = errMsg(e);
		}
	}

	const relativeUpdated = $derived(item ? new Date(item.updated_at).toLocaleString() : '');
	const relativeCreated = $derived(item ? new Date(item.created_at).toLocaleString() : '');
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

		{#if notFound}
			<div class="rounded-lg border border-line bg-card p-6 text-center sm:p-12">
				<p class="text-fg-muted">No item with that reference.</p>
				<p class="mt-1 text-xs text-fg-faint">It may have been deleted, or the link is wrong.</p>
			</div>
		{:else if loadError}
			<div class="rounded-md border border-danger/40 bg-danger/10 p-4">
				<p class="text-sm text-danger">{loadError}</p>
			</div>
		{:else if item === null || project === null}
			<p class="text-sm text-fg-faint">Loading…</p>
		{:else if !editing}
			<!-- READ MODE -->
			<header
				class="mb-6 border-b pb-5"
				style:border-bottom-color={projectColourVar(project.colour)}
			>
				<div class="mb-3 flex flex-wrap items-center gap-2">
					<span
						class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium"
						style={kindChipStyle(item.kind)}
					>
						{KIND_LABEL[item.kind]}
					</span>
					<span
						class="inline-flex items-center gap-1.5 rounded-full bg-card-2 px-2.5 py-0.5 text-xs font-medium text-fg-muted"
					>
						<StatusIcon status={item.status} />
						{STATUS_LABEL[item.status]}
					</span>
					<span class="font-mono text-xs text-fg-faint">#{item.sequence}</span>
					<span class="flex-1"></span>
					<button
						type="button"
						onclick={startEdit}
						aria-label="Edit item"
						title="Edit"
						class="rounded-md p-2 text-fg-muted transition hover:bg-card-2 hover:text-fg"
					>
						<Pencil class="h-4 w-4" />
					</button>
					<button
						type="button"
						onclick={() => (deleteConfirmOpen = true)}
						aria-label="Delete item"
						title="Delete"
						class="rounded-md p-2 text-fg-muted transition hover:bg-danger/10 hover:text-danger"
					>
						<Trash2 class="h-4 w-4" />
					</button>
				</div>
				<h1 class="text-2xl font-semibold tracking-tight text-fg">{item.title}</h1>
			</header>

			<ItemBody {item} />

			{#if attachmentsEnabled}
				<section class="mt-8 border-t border-line pt-6">
					<h2 class="mb-3 text-sm font-medium text-fg">Attachments</h2>
					<AttachmentUploader
						slug={project.slug}
						seq={item.sequence}
						onUploaded={refreshItemFromServer}
						onGooglePhotosClick={() => (gphotosOpen = true)}
					/>
					<div class="mt-3">
						<AttachmentList
							slug={project.slug}
							seq={item.sequence}
							attachments={item.attachments}
							onDeleted={refreshItemFromServer}
						/>
					</div>
				</section>
			{/if}

			<section class="mt-8 border-t border-line pt-6">
				<h2 class="mb-3 text-sm font-medium text-fg">Activity</h2>
				<ActivityRail slug={project.slug} itemId={item.id} refreshKey={item} />
			</section>

			<footer class="mt-10 flex flex-wrap gap-x-6 gap-y-1 text-xs text-fg-faint">
				<span>Created {relativeCreated}</span>
				<span>Updated {relativeUpdated}</span>
			</footer>
		{:else}
			<!-- EDIT MODE -->
			<form
				onsubmit={(e) => {
					e.preventDefault();
					saveEdit();
				}}
				class="flex flex-col gap-5"
			>
				<div class="flex flex-wrap items-center gap-2">
					<span class="text-xs font-medium tracking-wide text-fg-faint uppercase">Kind</span>
					<KindPicker value={editKind} onSelect={(k) => (editKind = k)} />
				</div>

				<div class="flex flex-wrap items-center gap-2">
					<span class="text-xs font-medium tracking-wide text-fg-faint uppercase">Status</span>
					<StatusPicker value={editStatus} onSelect={(s) => (editStatus = s)} />
				</div>

				<label class="flex flex-col gap-2">
					<span class="text-sm font-medium text-fg">Title</span>
					<input
						type="text"
						bind:value={editTitle}
						required
						maxlength={200}
						class="rounded-md border border-line bg-card px-3 py-2 text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
					/>
				</label>

				<label class="flex flex-col gap-2">
					<span class="text-sm font-medium text-fg"
						>Notes <span class="font-normal text-fg-faint">(markdown)</span></span
					>
					<textarea
						bind:value={editBody}
						rows="10"
						placeholder="Anything more to say?"
						class="resize-y rounded-md border border-line bg-card px-3 py-2 font-mono text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
					></textarea>
				</label>

				{#if saveError}
					<p class="text-sm text-danger">{saveError}</p>
				{/if}

				<div class="flex items-center gap-2">
					<button
						type="submit"
						disabled={saving || !editTitle.trim()}
						class="inline-flex items-center gap-1.5 rounded-md bg-accent px-4 py-2 text-sm font-medium text-on-accent transition hover:bg-accent-hover disabled:opacity-50"
					>
						<Check class="h-4 w-4" />
						{saving ? 'Saving…' : 'Save'}
					</button>
					<button
						type="button"
						onclick={cancelEdit}
						class="inline-flex items-center gap-1.5 rounded-md border border-line px-4 py-2 text-sm font-medium text-fg transition hover:bg-card-2"
					>
						<X class="h-4 w-4" />
						Cancel
					</button>
				</div>
			</form>
		{/if}

		{#if item && project}
			<ConfirmDialog
				bind:open={deleteConfirmOpen}
				title={`Delete item #${item.sequence}?`}
				message="This is permanent. The item and any future attachments / activity for it will be removed."
				confirmLabel="Delete item"
				cancelLabel="Keep it"
				destructive={true}
				onConfirm={performDelete}
			/>
			<GooglePhotosImportFlow
				bind:open={gphotosOpen}
				slug={project.slug}
				seq={item.sequence}
				onImported={refreshItemFromServer}
			/>
		{/if}
	</main>
{/if}
