<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { createTag, checkTagSlug, type Tag } from '$lib/api/tags';
	import { ApiError } from '$lib/api';
	import { goto } from '$app/navigation';
	import EmojiPicker from '$lib/components/EmojiPicker.svelte';
	import SlugField, { type SlugStatus } from '$lib/components/SlugField.svelte';
	import TagChip from '$lib/components/TagChip.svelte';
	import TagColourPicker from '$lib/components/TagColourPicker.svelte';
	import { ArrowLeft } from '@lucide/svelte';

	$effect(() => {
		if (!auth.loading && !auth.user) goto('/login');
	});

	let name = $state('');
	let slug = $state('');
	let slugStatus = $state<SlugStatus>('idle');
	let description = $state('');
	let icon = $state('');
	let colour = $state<string | null>(null);
	let error = $state<string | null>(null);
	let submitting = $state(false);

	const slugBlocksSubmit = $derived(
		slugStatus === 'invalid' || slugStatus === 'taken' || slugStatus === 'checking'
	);

	// Live preview — a dummy Tag we feed to TagChip.
	const previewTag = $derived<Tag>({
		id: 'preview',
		slug: slug.trim() || 'new-tag',
		name: name.trim() || 'New tag',
		description: null,
		icon: icon.trim() || null,
		colour,
		user_id: null,
		group_id: null,
		archived_at: null,
		created_at: new Date().toISOString(),
		updated_at: new Date().toISOString()
	});

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = null;
		submitting = true;
		try {
			const tag = await createTag({
				name: name.trim(),
				slug: slug.trim() || undefined,
				description: description.trim() || null,
				icon: icon.trim() || null,
				colour
			});
			goto(`/tags/${tag.slug}`);
		} catch (e) {
			error = e instanceof ApiError ? e.message : String(e);
			submitting = false;
		}
	}
</script>

{#if !auth.loading && auth.user}
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

		<h1 class="mb-2 text-2xl font-semibold tracking-tight text-fg">New tag</h1>
		<p class="mb-6 text-sm text-fg-muted">A label you can stick on items across your projects.</p>

		<div class="mb-8 rounded-lg border border-line bg-card-2 p-6 text-center">
			<p class="mb-3 text-xs font-medium tracking-wide text-fg-faint uppercase">Preview</p>
			<div class="flex justify-center">
				<TagChip tag={previewTag} />
			</div>
		</div>

		<form onsubmit={handleSubmit} class="flex flex-col gap-5">
			<label class="flex flex-col gap-2">
				<span class="text-sm font-medium text-fg">Name</span>
				<input
					type="text"
					bind:value={name}
					required
					maxlength={60}
					placeholder="e.g. bug, urgent, v1"
					class="rounded-md border border-line bg-card px-3 py-2 text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
				/>
			</label>

			<label class="flex flex-col gap-2">
				<span class="text-sm font-medium text-fg"
					>URL slug <span class="font-normal text-fg-faint">(optional)</span></span
				>
				<SlugField
					bind:value={slug}
					bind:status={slugStatus}
					{name}
					checkSlugFn={checkTagSlug}
					entityLabel="tag"
				/>
			</label>

			<div class="flex flex-col gap-2">
				<span class="text-sm font-medium text-fg"
					>Colour <span class="font-normal text-fg-faint">(optional)</span></span
				>
				<TagColourPicker bind:value={colour} />
			</div>

			<div class="flex flex-col gap-2">
				<span class="text-sm font-medium text-fg"
					>Icon <span class="font-normal text-fg-faint">(optional)</span></span
				>
				<EmojiPicker bind:value={icon} />
			</div>

			<label class="flex flex-col gap-2">
				<span class="text-sm font-medium text-fg"
					>Description <span class="font-normal text-fg-faint">(optional)</span></span
				>
				<textarea
					bind:value={description}
					maxlength={500}
					rows="3"
					placeholder="What is this tag for?"
					class="resize-none rounded-md border border-line bg-card px-3 py-2 text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
				></textarea>
			</label>

			<div class="flex items-center gap-3 pt-2">
				<button
					type="submit"
					disabled={submitting || !name.trim() || slugBlocksSubmit}
					class="rounded-md bg-accent px-4 py-2 text-sm font-medium text-on-accent transition hover:bg-accent-hover disabled:opacity-50"
				>
					{submitting ? 'Creating…' : 'Create tag'}
				</button>
				<a href="/tags" class="text-sm text-fg-muted hover:text-fg hover:underline">Cancel</a>
			</div>
		</form>

		{#if error}
			<div class="mt-6 rounded-md border border-danger/40 bg-danger/10 p-4">
				<p class="text-sm text-danger">{error}</p>
			</div>
		{/if}
	</main>
{/if}
