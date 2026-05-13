<script lang="ts">
	import type { TagWithCount } from '$lib/api/tags';
	import type { TagFilterMode } from '$lib/api/items';
	import { tagColourVar } from '$lib/tagColours';

	type Props = {
		tags: TagWithCount[];
		selectedSlugs: string[];
		mode: TagFilterMode;
		onToggle: (slug: string) => void;
		onModeChange: (mode: TagFilterMode) => void;
	};

	let { tags, selectedSlugs, mode, onToggle, onModeChange }: Props = $props();

	const selectedSet = $derived(new Set(selectedSlugs));
</script>

<aside class="rounded-lg border border-line bg-card p-4">
	<h2 class="mb-3 text-sm font-medium text-fg">Tags</h2>

	{#if tags.length === 0}
		<p class="text-xs text-fg-faint">
			No tags in this project yet. Add tags to items to filter here.
		</p>
	{:else}
		<fieldset class="mb-4">
			<legend class="mb-1.5 text-xs text-fg-muted">Match</legend>
			<label class="flex items-center gap-2 text-sm text-fg">
				<input
					type="radio"
					name="tag-mode"
					value="and"
					checked={mode === 'and'}
					onchange={() => onModeChange('and')}
				/>
				<span>all</span>
			</label>
			<label class="flex items-center gap-2 text-sm text-fg">
				<input
					type="radio"
					name="tag-mode"
					value="or"
					checked={mode === 'or'}
					onchange={() => onModeChange('or')}
				/>
				<span>any</span>
			</label>
		</fieldset>

		<ul class="flex flex-col gap-1">
			{#each tags as tag (tag.id)}
				<li>
					<label
						class="flex cursor-pointer items-center gap-2 rounded px-1 py-1 text-sm transition hover:bg-card-2"
					>
						<input
							type="checkbox"
							checked={selectedSet.has(tag.slug)}
							onchange={() => onToggle(tag.slug)}
							class="rounded"
						/>
						<span
							class="h-2.5 w-2.5 shrink-0 rounded-full"
							style:background-color={tagColourVar(tag.colour)}
						></span>
						<span class="min-w-0 flex-1 truncate text-fg">{tag.name}</span>
						<span class="text-xs text-fg-faint">{tag.item_count}</span>
					</label>
				</li>
			{/each}
		</ul>
	{/if}
</aside>
