<script lang="ts">
	import { ITEM_KINDS, type ItemKind } from '$lib/api/items';
	import { KIND_LABEL, kindChipStyle } from '$lib/itemDisplay';

	type Props = {
		value: ItemKind;
		onSelect: (k: ItemKind) => void;
		disabled?: boolean;
	};

	let { value, onSelect, disabled = false }: Props = $props();
</script>

<div class="flex flex-wrap gap-1.5">
	{#each ITEM_KINDS as k (k)}
		<button
			type="button"
			onclick={() => onSelect(k)}
			{disabled}
			style={kindChipStyle(k, value === k)}
			class="rounded-full border-2 px-3 py-1 text-xs font-medium transition disabled:cursor-not-allowed disabled:opacity-50"
			class:border-transparent={value !== k}
		>
			{KIND_LABEL[k]}
		</button>
	{/each}
</div>
