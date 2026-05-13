<script lang="ts">
	import type { Item } from '$lib/api/items';
	import type { Tag } from '$lib/api/tags';
	import TagChip from './TagChip.svelte';
	import { X } from '@lucide/svelte';

	type Props = {
		tag: Tag;
		items: Item[];
		onCancel: () => void;
		onConfirm: () => void | Promise<void>;
		deleting?: boolean;
	};

	let { tag, items, onCancel, onConfirm, deleting = false }: Props = $props();

	const previewItems = $derived(items.slice(0, 4));
	const remaining = $derived(Math.max(items.length - previewItems.length, 0));
</script>

<div
	role="dialog"
	aria-modal="true"
	aria-labelledby="delete-tag-title"
	class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
>
	<div class="w-full max-w-md rounded-lg border border-line bg-card shadow-xl">
		<div class="flex items-start justify-between p-5 pb-3">
			<h2 id="delete-tag-title" class="text-lg font-semibold text-fg">
				Delete tag?
			</h2>
			<button
				type="button"
				onclick={onCancel}
				aria-label="Cancel"
				class="rounded p-1 text-fg-muted hover:bg-card-2 hover:text-fg"
			>
				<X class="h-4 w-4" />
			</button>
		</div>

		<div class="px-5 pb-5">
			<div class="mb-4 flex items-center gap-2">
				<TagChip {tag} />
			</div>

			{#if items.length === 0}
				<p class="text-sm text-fg-muted">
					This tag isn't used on any items yet. Deleting it is safe.
				</p>
			{:else}
				<p class="mb-3 text-sm text-fg-muted">
					This tag is on {items.length}
					{items.length === 1 ? 'item' : 'items'}:
				</p>
				<ul class="mb-4 space-y-1 rounded-md bg-card-2 p-3 text-sm">
					{#each previewItems as item (item.id)}
						<li class="truncate text-fg">
							<span class="font-mono text-xs text-fg-faint">#{item.sequence}</span>
							{item.title}
						</li>
					{/each}
					{#if remaining > 0}
						<li class="text-xs text-fg-faint">+ {remaining} more</li>
					{/if}
				</ul>
				<p class="text-sm text-fg-muted">
					The tag will be removed from all of them. Items themselves are not deleted.
				</p>
			{/if}
		</div>

		<div class="flex items-center justify-end gap-2 border-t border-line px-5 py-3">
			<button
				type="button"
				onclick={onCancel}
				disabled={deleting}
				class="rounded-md px-3 py-1.5 text-sm text-fg-muted transition hover:bg-card-2 hover:text-fg disabled:opacity-50"
			>
				Cancel
			</button>
			<button
				type="button"
				onclick={onConfirm}
				disabled={deleting}
				class="rounded-md bg-danger px-3 py-1.5 text-sm font-medium text-white transition hover:brightness-110 disabled:opacity-50"
			>
				{deleting ? 'Deleting…' : 'Delete'}
			</button>
		</div>
	</div>
</div>
