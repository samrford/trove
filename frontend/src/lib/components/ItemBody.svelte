<script lang="ts">
	import type { Item } from '$lib/api/items';
	import { markdownBody } from '$lib/markdown';

	type Props = {
		item: Item;
		class?: string;
	};

	let { item, class: cls = '' }: Props = $props();
</script>

{#if item.body}
	<article
		use:markdownBody={item.body}
		class="prose prose-sm prose-invert max-w-none text-fg {cls}"
	></article>
{:else}
	<p class="text-sm italic text-fg-faint {cls}">No notes yet.</p>
{/if}
