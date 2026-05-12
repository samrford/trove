<script lang="ts">
	import { ITEM_STATUSES, type ItemStatus } from '$lib/api/items';
	import { STATUS_LABEL, statusChipStyle } from '$lib/itemDisplay';
	import StatusIcon from './StatusIcon.svelte';

	type Props = {
		value: ItemStatus;
		onSelect: (s: ItemStatus) => void;
		disabled?: boolean;
		// 'sm' fits a tight footer row, 'md' fits a form.
		size?: 'sm' | 'md';
	};

	let { value, onSelect, disabled = false, size = 'md' }: Props = $props();
</script>

<div class="flex flex-wrap gap-1.5">
	{#each ITEM_STATUSES as s (s)}
		<button
			type="button"
			onclick={() => onSelect(s)}
			{disabled}
			style={statusChipStyle(value === s)}
			class="inline-flex items-center gap-1 rounded-full border-2 font-medium transition disabled:cursor-not-allowed disabled:opacity-50"
			class:px-2={size === 'sm'}
			class:py-0.5={size === 'sm'}
			class:text-xs={true}
			class:px-3={size === 'md'}
			class:py-1={size === 'md'}
			class:border-transparent={value !== s}
		>
			<StatusIcon status={s} class={size === 'sm' ? 'h-3 w-3' : 'h-3.5 w-3.5'} />
			{STATUS_LABEL[s]}
		</button>
	{/each}
</div>
