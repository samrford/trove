<script lang="ts">
	import type { TagWithCount } from '$lib/api/tags';
	import type { TagFilterMode } from '$lib/api/items';
	import { tagColourVar } from '$lib/tagColours';
	import { ListFilter } from '@lucide/svelte';
	import Popover from './Popover.svelte';

	type Props = {
		tags: TagWithCount[];
		selectedSlugs: string[];
		mode: TagFilterMode;
		onToggle: (slug: string) => void;
		onModeChange: (mode: TagFilterMode) => void;
	};

	let { tags, selectedSlugs, mode, onToggle, onModeChange }: Props = $props();

	const selectedSet = $derived(new Set(selectedSlugs));
	const activeCount = $derived(selectedSlugs.length);
</script>

<Popover label="Filter by tags">
	{#snippet trigger({ open, toggle })}
		<button
			type="button"
			onclick={toggle}
			aria-haspopup="dialog"
			aria-expanded={open}
			class="inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium transition"
			class:bg-fg={activeCount > 0}
			class:text-on-accent={activeCount > 0}
			class:text-fg-muted={activeCount === 0}
			class:hover:bg-card-2={activeCount === 0}
		>
			<ListFilter class="h-3.5 w-3.5" />
			Filter
			{#if activeCount > 0}
				<span
					class="ml-0.5 inline-flex h-4 min-w-4 items-center justify-center rounded-full bg-on-accent/20 px-1 text-[10px] font-semibold"
				>
					{activeCount}
				</span>
			{/if}
		</button>
	{/snippet}

	<div class="w-64 p-2">
		{#if tags.length === 0}
			<p class="px-1 py-2 text-xs text-fg-faint">
				No tags in this project yet. Add tags to items to filter here.
			</p>
		{:else}
			<fieldset class="mb-3 border-b border-line/60 pb-3">
				<legend class="mb-1.5 text-xs text-fg-muted">Match</legend>
				<div class="flex gap-4">
					<label class="flex items-center gap-2 text-sm text-fg">
						<input
							type="radio"
							name="tag-mode-popover"
							value="and"
							checked={mode === 'and'}
							onchange={() => onModeChange('and')}
						/>
						<span>all</span>
					</label>
					<label class="flex items-center gap-2 text-sm text-fg">
						<input
							type="radio"
							name="tag-mode-popover"
							value="or"
							checked={mode === 'or'}
							onchange={() => onModeChange('or')}
						/>
						<span>any</span>
					</label>
				</div>
			</fieldset>

			<ul class="flex max-h-64 flex-col gap-0.5 overflow-y-auto">
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
	</div>
</Popover>
