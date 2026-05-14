<script lang="ts">
	import type { Tag } from '$lib/api/tags';
	import { tagColourVar } from '$lib/tagColours';
	import { X } from '@lucide/svelte';

	type Props = {
		tag: Tag;
		size?: 'sm' | 'md';
		// When provided, the chip body is a button that fires this.
		onClick?: (e: MouseEvent) => void;
		// When provided, renders an × button on the right that fires this.
		onRemove?: () => void;
	};

	let { tag, size = 'md', onClick, onRemove }: Props = $props();

	const colour = $derived(tagColourVar(tag.colour));
	// Soft-tinted background (~14% mix of tag colour) + full-saturation text.
	const bg = $derived(`oklch(from ${colour} l c h / 0.14)`);
	const padding = $derived(size === 'sm' ? 'px-1.5 py-0.5 text-xs' : 'px-2 py-0.5 text-sm');
	const innerGap = $derived(size === 'sm' ? 'gap-1' : 'gap-1.5');
</script>

<span
	class="inline-flex items-center rounded-md font-medium {padding}"
	style:background-color={bg}
	style:color={colour}
>
	{#if onClick}
		<button
			type="button"
			onclick={onClick}
			class="flex cursor-pointer items-center transition hover:brightness-110 {innerGap}"
			style:color={colour}
		>
			{#if tag.icon}<span aria-hidden="true">{tag.icon}</span>{/if}
			<span>{tag.name}</span>
		</button>
	{:else}
		<span class="flex items-center {innerGap}">
			{#if tag.icon}<span aria-hidden="true">{tag.icon}</span>{/if}
			<span>{tag.name}</span>
		</span>
	{/if}

	{#if onRemove}
		<button
			type="button"
			onclick={() => onRemove?.()}
			aria-label="Remove {tag.name}"
			class="-mr-0.5 ml-1 inline-flex cursor-pointer items-center justify-center rounded p-0.5 hover:bg-card-2"
		>
			<X class="h-3 w-3" />
		</button>
	{/if}
</span>
