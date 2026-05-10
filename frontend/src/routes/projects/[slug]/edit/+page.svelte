<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { getProject, updateProject, deleteProject, type Project } from '$lib/api/projects';
	import { ApiError } from '$lib/api';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import AppHeader from '$lib/components/AppHeader.svelte';
	import ColourPicker from '$lib/components/ColourPicker.svelte';
	import EmojiPicker from '$lib/components/EmojiPicker.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import SlugField, { type SlugStatus } from '$lib/components/SlugField.svelte';
	import { ArrowLeft, Trash2 } from '@lucide/svelte';

	$effect(() => {
		if (!auth.loading && !auth.user) goto('/login');
	});

	let project = $state<Project | null>(null);
	let loadError = $state<string | null>(null);

	let name = $state('');
	let originalName = $state('');
	let slug = $state('');
	let originalSlug = $state('');
	let slugStatus = $state<SlugStatus>('idle');
	let description = $state('');
	let icon = $state('');
	let colour = $state<string | null>(null);

	let saveError = $state<string | null>(null);
	let saving = $state(false);
	let deleting = $state(false);
	let deleteConfirmOpen = $state(false);

	const slugBlocksSubmit = $derived(
		slugStatus === 'invalid' || slugStatus === 'taken' || slugStatus === 'checking'
	);

	$effect(() => {
		const slugParam = page.params.slug;
		if (auth.user && slugParam && project === null) {
			getProject(slugParam)
				.then((res) => {
					project = res;
					name = res.name;
					originalName = res.name;
					slug = res.slug;
					originalSlug = res.slug;
					description = res.description ?? '';
					icon = res.icon ?? '';
					colour = res.colour;
				})
				.catch((e) => {
					loadError = e instanceof ApiError ? e.message : String(e);
				});
		}
	});

	async function handleSave(e: SubmitEvent) {
		e.preventDefault();
		if (!project) return;
		saveError = null;
		saving = true;
		try {
			const updated = await updateProject(project.slug, {
				name: name.trim(),
				slug: slug.trim() === originalSlug ? undefined : slug.trim(),
				description: description.trim() || null,
				icon: icon.trim() || null,
				colour
			});
			goto(`/projects/${updated.slug}`);
		} catch (e) {
			saveError = e instanceof ApiError ? e.message : String(e);
			saving = false;
		}
	}

	async function performDelete() {
		if (!project) return;
		deleting = true;
		try {
			await deleteProject(project.slug);
			goto('/');
		} catch (e) {
			saveError = e instanceof ApiError ? e.message : String(e);
			deleting = false;
		}
	}
</script>

{#if !auth.loading && auth.user}
	<AppHeader />
	<main class="mx-auto max-w-2xl px-6 py-10">
		<div class="mb-8">
			<a
				href={project ? `/projects/${project.slug}` : '/'}
				class="inline-flex items-center gap-1.5 text-sm text-fg-muted hover:text-fg hover:underline"
			>
				<ArrowLeft class="h-4 w-4" />
				Back to project
			</a>
		</div>

		{#if loadError}
			<div class="rounded-md border border-danger/40 bg-danger/10 p-4">
				<p class="text-sm text-danger">{loadError}</p>
			</div>
		{:else if project === null}
			<p class="text-sm text-fg-faint">Loading…</p>
		{:else}
			<h1 class="mb-8 text-2xl font-semibold tracking-tight text-fg">Edit project</h1>

			<form onsubmit={handleSave} class="flex flex-col gap-5">
				<label class="flex flex-col gap-2">
					<span class="text-sm font-medium text-fg">Name</span>
					<input
						type="text"
						bind:value={name}
						required
						maxlength={120}
						class="rounded-md border border-line bg-card px-3 py-2 text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
					/>
				</label>

				<label class="flex flex-col gap-2">
					<span class="text-sm font-medium text-fg">URL slug</span>
					<SlugField
						bind:value={slug}
						bind:status={slugStatus}
						{name}
						{originalName}
						{originalSlug}
					/>
				</label>

				<label class="flex flex-col gap-2">
					<span class="text-sm font-medium text-fg"
						>Description <span class="font-normal text-fg-faint">(optional)</span></span
					>
					<textarea
						bind:value={description}
						maxlength={500}
						rows="3"
						class="resize-none rounded-md border border-line bg-card px-3 py-2 text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
					></textarea>
				</label>

				<div class="flex flex-col gap-2">
					<span class="text-sm font-medium text-fg"
						>Colour <span class="font-normal text-fg-faint">(optional)</span></span
					>
					<ColourPicker bind:value={colour} />
				</div>

				<div class="flex flex-col gap-2">
					<span class="text-sm font-medium text-fg"
						>Icon <span class="font-normal text-fg-faint">(optional)</span></span
					>
					<EmojiPicker bind:value={icon} />
				</div>

				<div class="flex items-center gap-3 pt-2">
					<button
						type="submit"
						disabled={saving || deleting || !name.trim() || slugBlocksSubmit}
						class="rounded-md bg-accent px-4 py-2 text-sm font-medium text-on-accent transition hover:bg-accent-hover disabled:opacity-50"
					>
						{saving ? 'Saving…' : 'Save changes'}
					</button>
					<a
						href={`/projects/${project.slug}`}
						class="text-sm text-fg-muted hover:text-fg hover:underline">Cancel</a
					>
				</div>
			</form>

			{#if saveError}
				<div class="mt-6 rounded-md border border-danger/40 bg-danger/10 p-4">
					<p class="text-sm text-danger">{saveError}</p>
				</div>
			{/if}

			<section class="mt-12 rounded-md border border-danger/40 bg-danger/5 p-5">
				<h2 class="text-sm font-semibold text-danger">Danger zone</h2>
				<p class="mt-1 text-sm text-fg-muted">
					Deleting a project removes it permanently, along with all of its items, tags, attachments,
					and activity. This can't be undone.
				</p>
				<button
					type="button"
					onclick={() => (deleteConfirmOpen = true)}
					disabled={saving || deleting}
					class="mt-4 inline-flex items-center gap-1.5 rounded-md border border-danger/60 px-4 py-2 text-sm font-medium text-danger transition hover:bg-danger/10 disabled:opacity-50"
				>
					<Trash2 class="h-4 w-4" />
					{deleting ? 'Deleting…' : 'Delete project'}
				</button>
			</section>

			<ConfirmDialog
				bind:open={deleteConfirmOpen}
				title={`Delete "${project.name}"?`}
				message="This is permanent. The project and all of its items, tags, attachments, and activity will be removed."
				confirmLabel="Delete project"
				cancelLabel="Keep it"
				destructive={true}
				onConfirm={performDelete}
			/>
		{/if}
	</main>
{/if}
