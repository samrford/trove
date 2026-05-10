<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { createProject } from '$lib/api/projects';
	import { ApiError } from '$lib/api';
	import { goto } from '$app/navigation';
	import AppHeader from '$lib/components/AppHeader.svelte';
	import ColourPicker from '$lib/components/ColourPicker.svelte';
	import EmojiPicker from '$lib/components/EmojiPicker.svelte';
	import SlugField, { type SlugStatus } from '$lib/components/SlugField.svelte';
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

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = null;
		submitting = true;
		try {
			const project = await createProject({
				name: name.trim(),
				slug: slug.trim() || undefined,
				description: description.trim() || null,
				icon: icon.trim() || null,
				colour
			});
			goto(`/projects/${project.slug}`);
		} catch (e) {
			error = e instanceof ApiError ? e.message : String(e);
			submitting = false;
		}
	}
</script>

{#if !auth.loading && auth.user}
	<AppHeader />
	<main class="mx-auto max-w-2xl px-6 py-10">
		<div class="mb-8">
			<a
				href="/"
				class="inline-flex items-center gap-1.5 text-sm text-fg-muted hover:text-fg hover:underline"
			>
				<ArrowLeft class="h-4 w-4" />
				Back
			</a>
		</div>

		<h1 class="mb-2 text-2xl font-semibold tracking-tight text-fg">New project</h1>
		<p class="mb-8 text-sm text-fg-muted">
			Give it a name and a one-liner. You can flesh out the rest later.
		</p>

		<form onsubmit={handleSubmit} class="flex flex-col gap-5">
			<label class="flex flex-col gap-2">
				<span class="text-sm font-medium text-fg">Name</span>
				<input
					type="text"
					bind:value={name}
					required
					maxlength={120}
					autofocus
					placeholder="e.g. Recipes, Side hustle, Trove itself"
					class="rounded-md border border-line bg-card px-3 py-2 text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
				/>
			</label>

			<label class="flex flex-col gap-2">
				<span class="text-sm font-medium text-fg"
					>URL slug <span class="font-normal text-fg-faint">(optional)</span></span
				>
				<SlugField bind:value={slug} bind:status={slugStatus} {name} />
			</label>

			<label class="flex flex-col gap-2">
				<span class="text-sm font-medium text-fg"
					>Description <span class="font-normal text-fg-faint">(optional)</span></span
				>
				<textarea
					bind:value={description}
					maxlength={500}
					rows="3"
					placeholder="What's this project for?"
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
					disabled={submitting || !name.trim() || slugBlocksSubmit}
					class="rounded-md bg-accent px-4 py-2 text-sm font-medium text-on-accent transition hover:bg-accent-hover disabled:opacity-50"
				>
					{submitting ? 'Creating…' : 'Create project'}
				</button>
				<a href="/" class="text-sm text-fg-muted hover:text-fg hover:underline">Cancel</a>
			</div>
		</form>

		{#if error}
			<div class="mt-6 rounded-md border border-danger/40 bg-danger/10 p-4">
				<p class="text-sm text-danger">{error}</p>
			</div>
		{/if}
	</main>
{/if}
