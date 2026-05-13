<script lang="ts">
	import type { Tag } from '$lib/api/tags';
	import TagChip from './TagChip.svelte';
	import Popover from './Popover.svelte';
	import { ExternalLink, X } from '@lucide/svelte';

	// Renders a TagChip whose body click opens a small popover with actions.
	// Used on item cards in the project view.

	type Props = {
		tag: Tag;
		onRemove?: () => void | Promise<void>;
	};

	let { tag, onRemove }: Props = $props();
</script>

<Popover label="Tag actions">
	{#snippet trigger({ toggle })}
		<TagChip {tag} size="sm" onClick={() => toggle()} />
	{/snippet}

	{#snippet children({ close })}
		<div class="flex w-44 flex-col text-sm">
			<a
				href={`/tags/${tag.slug}`}
				onclick={() => close()}
				class="flex items-center gap-2 rounded px-2 py-1.5 text-fg transition hover:bg-card-2"
			>
				<ExternalLink class="h-3.5 w-3.5 text-fg-muted" />
				<span>Open tag</span>
			</a>
			{#if onRemove}
				<button
					type="button"
					onclick={async () => {
						await onRemove?.();
						close();
					}}
					class="flex items-center gap-2 rounded px-2 py-1.5 text-left text-fg transition hover:bg-card-2"
				>
					<X class="h-3.5 w-3.5 text-fg-muted" />
					<span>Remove from this item</span>
				</button>
			{/if}
		</div>
	{/snippet}
</Popover>
