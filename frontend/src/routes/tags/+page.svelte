<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { listTags, type TagWithCount } from '$lib/api/tags';
	import { errMsg } from '$lib/api';
	import { goto } from '$app/navigation';
	import TagChip from '$lib/components/TagChip.svelte';
	import { Plus, Tag as TagIcon } from '@lucide/svelte';

	let tags = $state<TagWithCount[] | null>(null);
	let error = $state<string | null>(null);

	$effect(() => {
		if (!auth.loading && !auth.user) goto('/login');
	});

	$effect(() => {
		if (auth.user && tags === null) {
			listTags()
				.then((res) => (tags = res))
				.catch((e) => {
					error = errMsg(e);
				});
		}
	});
</script>

{#if !auth.loading && auth.user}
	<main class="mx-auto max-w-5xl px-6 py-10">
		<div class="mb-8 flex items-center justify-between">
			<div>
				<h1 class="text-2xl font-semibold tracking-tight text-fg">Tags</h1>
				<p class="mt-1 text-sm text-fg-muted">
					Labels you can apply to items across your projects.
				</p>
			</div>
			{#if tags && tags.length > 0}
				<a
					href="/tags/new"
					class="inline-flex items-center gap-1.5 rounded-md bg-accent px-4 py-2 text-sm font-medium text-on-accent transition hover:bg-accent-hover"
				>
					<Plus class="h-4 w-4" />
					New tag
				</a>
			{/if}
		</div>

		{#if error}
			<div class="mb-4 rounded-md border border-danger/40 bg-danger/10 p-4">
				<p class="text-sm text-danger">{error}</p>
			</div>
		{/if}

		{#if tags === null && !error}
			<p class="text-sm text-fg-faint">Loading…</p>
		{:else if tags && tags.length === 0}
			<div class="rounded-lg border border-line bg-card p-6 text-center sm:p-12">
				<TagIcon class="mx-auto h-10 w-10 text-fg-faint" />
				<h2 class="mt-4 text-lg font-medium text-fg">No tags yet</h2>
				<p class="mx-auto mt-2 max-w-md text-sm text-fg-muted">
					Tags help you organise items across all your projects.
				</p>
				<a
					href="/tags/new"
					class="mt-6 inline-flex items-center gap-1.5 rounded-md bg-accent px-4 py-2 text-sm font-medium text-on-accent transition hover:bg-accent-hover"
				>
					<Plus class="h-4 w-4" />
					Create your first tag
				</a>
			</div>
		{:else if tags}
			<ul class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each tags as tag (tag.id)}
					<li>
						<a
							href={`/tags/${tag.slug}`}
							class="block rounded-lg border border-line bg-card p-4 transition hover:border-line-strong"
						>
							<div class="flex items-start justify-between gap-3">
								<TagChip {tag} />
								<span class="shrink-0 text-xs text-fg-faint">
									{tag.item_count}
									{tag.item_count === 1 ? 'item' : 'items'}
								</span>
							</div>
							{#if tag.description}
								<p class="mt-3 line-clamp-2 text-sm text-fg-muted">{tag.description}</p>
							{/if}
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</main>
{/if}
