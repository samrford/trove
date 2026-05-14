<script lang="ts">
	import { PROJECT_EMOJIS } from '$lib/emojis';
	import { ChevronDown } from '@lucide/svelte';
	import Popover from './Popover.svelte';

	type Props = { value?: string };
	let { value = $bindable('') }: Props = $props();
</script>

<Popover label="Choose an icon">
	{#snippet trigger({ open, toggle })}
		<button
			type="button"
			onclick={toggle}
			aria-label={value ? `Icon: ${value}. Change icon` : 'Choose icon'}
			aria-haspopup="dialog"
			aria-expanded={open}
			class="flex h-10 items-center gap-1.5 rounded-md border-2 bg-card px-2 transition hover:bg-card-2"
			class:border-fg={open}
			class:border-line={!open}
		>
			{#if value}
				<span class="flex h-6 w-6 items-center justify-center text-2xl leading-none">{value}</span>
			{:else}
				<span class="flex h-6 w-6 items-center justify-center text-xs text-fg-faint">—</span>
			{/if}
			<ChevronDown class="h-4 w-4 text-fg-faint" />
		</button>
	{/snippet}

	{#snippet children({ close })}
		<div class="flex max-w-xs flex-wrap gap-1 sm:max-w-md">
			<button
				type="button"
				onclick={() => {
					value = '';
					close();
				}}
				aria-label="No icon"
				aria-pressed={value === ''}
				class="flex h-10 w-10 items-center justify-center rounded-md border-2 transition"
				class:border-fg={value === ''}
				class:border-line={value !== ''}
			>
				<span class="text-xs text-fg-faint">—</span>
			</button>
			{#each PROJECT_EMOJIS as emoji (emoji)}
				<button
					type="button"
					onclick={() => {
						value = emoji;
						close();
					}}
					aria-label={emoji}
					aria-pressed={value === emoji}
					class="flex h-10 w-10 items-center justify-center rounded-md border-2 transition hover:bg-card-2"
					class:border-fg={value === emoji}
					class:border-transparent={value !== emoji}
				>
					<span class="text-2xl leading-none">{emoji}</span>
				</button>
			{/each}
		</div>
	{/snippet}
</Popover>
