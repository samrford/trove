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
		class="prose prose-sm max-w-none text-fg prose-invert {cls}"
	></article>
{:else}
	<p class="text-sm text-fg-faint italic {cls}">No notes yet.</p>
{/if}
