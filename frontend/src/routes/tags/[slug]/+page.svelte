<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import {
		getTag,
		updateTag,
		deleteTag,
		listItemsForTag,
		checkTagSlug,
		type Tag
	} from '$lib/api/tags';
	import type { Item } from '$lib/api/items';
	import { ApiError } from '$lib/api';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import AppHeader from '$lib/components/AppHeader.svelte';
	import EmojiPicker from '$lib/components/EmojiPicker.svelte';
	import SlugField, { type SlugStatus } from '$lib/components/SlugField.svelte';
	import TagChip from '$lib/components/TagChip.svelte';
	import TagColourPicker from '$lib/components/TagColourPicker.svelte';
	import DeleteTagModal from '$lib/components/DeleteTagModal.svelte';
	import { slugify } from '$lib/slug';
	import { ArrowLeft } from '@lucide/svelte';

	$effect(() => {
		if (!auth.loading && !auth.user) goto('/login');
	});

	const slugFromUrl = $derived(page.params.slug);

	let tag = $state<Tag | null>(null);
	let items = $state<Item[]>([]);
	let loadError = $state<string | null>(null);

	let name = $state('');
	let slug = $state('');
	let slugStatus = $state<SlugStatus>('idle');
	let updateSlugWithName = $state(true);
	let description = $state('');
	let icon = $state('');
	let colour = $state<string | null>(null);

	let saving = $state(false);
	let saveError = $state<string | null>(null);
	let confirmingDelete = $state(false);
	let deleting = $state(false);

	$effect(() => {
		if (!auth.user || !slugFromUrl) return;
		let cancelled = false;
		Promise.all([getTag(slugFromUrl), listItemsForTag(slugFromUrl)])
			.then(([t, its]) => {
				if (cancelled) return;
				tag = t;
				items = its;
				name = t.name;
				slug = t.slug;
				description = t.description ?? '';
				icon = t.icon ?? '';
				colour = t.colour;
			})
			.catch((e) => {
				if (cancelled) return;
				loadError = e instanceof ApiError ? e.message : String(e);
			});
		return () => {
			cancelled = true;
		};
	});

	// Skip on initial load (when name === tag.name) so a manually-edited slug
	// isn't overwritten before the user starts typing.
	$effect(() => {
		if (updateSlugWithName && tag && name !== tag.name) {
			slug = slugify(name);
		}
	});

	const previewTag = $derived<Tag>(
		tag
			? {
					...tag,
					name: name.trim() || tag.name,
					slug: slug.trim() || tag.slug,
					icon: icon.trim() || null,
					colour
				}
			: {
					id: 'preview',
					slug: 'tag',
					name: 'Tag',
					description: null,
					icon: null,
					colour: null,
					user_id: null,
					group_id: null,
					archived_at: null,
					created_at: new Date().toISOString(),
					updated_at: new Date().toISOString()
				}
	);

	const slugBlocksSubmit = $derived(
		slugStatus === 'invalid' || slugStatus === 'taken' || slugStatus === 'checking'
	);
	const nothingToSave = $derived(
		tag &&
			name === tag.name &&
			slug === tag.slug &&
			description === (tag.description ?? '') &&
			icon === (tag.icon ?? '') &&
			colour === tag.colour
	);

	async function handleSave(e: SubmitEvent) {
		e.preventDefault();
		if (!tag) return;
		saveError = null;
		saving = true;
		try {
			const updated = await updateTag(tag.slug, {
				name: name.trim(),
				slug: slug.trim() || undefined,
				description: description.trim() || null,
				icon: icon.trim() || null,
				colour
			});
			tag = updated;
			if (updated.slug !== slugFromUrl) {
				goto(`/tags/${updated.slug}`, { replaceState: true });
			}
		} catch (e) {
			saveError = e instanceof ApiError ? e.message : String(e);
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!tag) return;
		deleting = true;
		try {
			await deleteTag(tag.slug);
			goto('/tags');
		} catch (e) {
			saveError = e instanceof ApiError ? e.message : String(e);
			deleting = false;
			confirmingDelete = false;
		}
	}
</script>

{#if !auth.loading && auth.user}
	<AppHeader />
	<main class="mx-auto max-w-2xl px-6 py-10">
		<div class="mb-8">
			<a
				href="/tags"
				class="inline-flex items-center gap-1.5 text-sm text-fg-muted hover:text-fg hover:underline"
			>
				<ArrowLeft class="h-4 w-4" />
				Back to tags
			</a>
		</div>

		{#if loadError}
			<div class="rounded-md border border-danger/40 bg-danger/10 p-4">
				<p class="text-sm text-danger">{loadError}</p>
			</div>
		{:else if !tag}
			<p class="text-sm text-fg-faint">Loading…</p>
		{:else}
			<h1 class="mb-6 text-2xl font-semibold tracking-tight text-fg">Edit tag</h1>

			<div class="mb-8 rounded-lg border border-line bg-card-2 p-6 text-center">
				<p class="mb-3 text-xs font-medium tracking-wide text-fg-faint uppercase">Preview</p>
				<div class="flex justify-center">
					<TagChip tag={previewTag} />
				</div>
			</div>

			<form onsubmit={handleSave} class="flex flex-col gap-5">
				<label class="flex flex-col gap-2">
					<span class="text-sm font-medium text-fg">Name</span>
					<input
						type="text"
						bind:value={name}
						required
						maxlength={60}
						class="rounded-md border border-line bg-card px-3 py-2 text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
					/>
				</label>

				<div class="flex flex-col gap-2">
					<span class="text-sm font-medium text-fg">URL slug</span>
					<SlugField
						bind:value={slug}
						bind:status={slugStatus}
						{name}
						originalName={tag.name}
						originalSlug={tag.slug}
						checkSlugFn={checkTagSlug}
						entityLabel="tag"
					/>
					<label class="mt-1 flex items-center gap-2 text-xs text-fg-muted">
						<input type="checkbox" bind:checked={updateSlugWithName} class="rounded" />
						<span>Update slug to match name</span>
					</label>
					{#if !updateSlugWithName && slug !== tag.slug}
						<p class="text-xs text-fg-faint">
							Slug stays as <code class="font-mono">{tag.slug}</code> until you change it manually.
						</p>
					{/if}
				</div>

				<div class="flex flex-col gap-2">
					<span class="text-sm font-medium text-fg">Colour</span>
					<TagColourPicker bind:value={colour} />
				</div>

				<div class="flex flex-col gap-2">
					<span class="text-sm font-medium text-fg">Icon</span>
					<EmojiPicker bind:value={icon} />
				</div>

				<label class="flex flex-col gap-2">
					<span class="text-sm font-medium text-fg">Description</span>
					<textarea
						bind:value={description}
						maxlength={500}
						rows="3"
						class="resize-none rounded-md border border-line bg-card px-3 py-2 text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
					></textarea>
				</label>

				<div class="flex items-center justify-between gap-3 pt-2">
					<button
						type="submit"
						disabled={saving || !name.trim() || slugBlocksSubmit || nothingToSave}
						class="rounded-md bg-accent px-4 py-2 text-sm font-medium text-on-accent transition hover:bg-accent-hover disabled:opacity-50"
					>
						{saving ? 'Saving…' : 'Save'}
					</button>
					<button
						type="button"
						onclick={() => (confirmingDelete = true)}
						class="rounded-md border border-danger/30 px-3 py-1.5 text-sm text-danger transition hover:bg-danger/10"
					>
						Delete
					</button>
				</div>
			</form>

			{#if saveError}
				<div class="mt-6 rounded-md border border-danger/40 bg-danger/10 p-4">
					<p class="text-sm text-danger">{saveError}</p>
				</div>
			{/if}

			<section class="mt-10">
				<h2 class="mb-4 text-sm font-medium tracking-wide text-fg-muted uppercase">
					{items.length}
					{items.length === 1 ? 'item using this tag' : 'items using this tag'}
				</h2>
				{#if items.length === 0}
					<p class="text-sm text-fg-faint">No items have this tag yet.</p>
				{:else}
					<ul class="flex flex-col gap-2">
						{#each items as item (item.id)}
							<li class="rounded-md border border-line bg-card px-3 py-2 text-sm text-fg">
								<span class="font-mono text-xs text-fg-faint">#{item.sequence}</span>
								{item.title}
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		{/if}
	</main>

	{#if confirmingDelete && tag}
		<DeleteTagModal
			{tag}
			{items}
			{deleting}
			onCancel={() => (confirmingDelete = false)}
			onConfirm={handleDelete}
		/>
	{/if}
{/if}
