<script lang="ts">
	import {
		PROJECT_COLOUR_NAMES,
		PROJECT_COLOUR_LABELS,
		projectColourVar,
		type ProjectColour
	} from '$lib/projectColours';
	import { ChevronDown } from '@lucide/svelte';
	import Popover from './Popover.svelte';

	type Props = { value?: string | null };
	let { value = $bindable(null) }: Props = $props();

	function labelFor(c: string | null): string | undefined {
		if (c && (PROJECT_COLOUR_NAMES as readonly string[]).includes(c)) {
			return PROJECT_COLOUR_LABELS[c as ProjectColour];
		}
		return undefined;
	}
</script>

<Popover label="Choose a colour">
	{#snippet trigger({ open, toggle })}
		<button
			type="button"
			onclick={toggle}
			aria-label={value ? `Colour: ${labelFor(value)}. Change colour` : 'Choose colour'}
			aria-haspopup="dialog"
			aria-expanded={open}
			class="flex h-10 items-center gap-1.5 rounded-md border-2 bg-card px-2 transition hover:bg-card-2"
			class:border-fg={open}
			class:border-line={!open}
		>
			{#if value}
				<span class="h-6 w-6 rounded-full" style:background-color={projectColourVar(value)}></span>
			{:else}
				<span class="flex h-6 w-6 items-center justify-center text-xs text-fg-faint">—</span>
			{/if}
			<ChevronDown class="h-4 w-4 text-fg-faint" />
		</button>
	{/snippet}

	{#snippet children({ close })}
		<div class="flex flex-wrap gap-2">
			<button
				type="button"
				onclick={() => {
					value = null;
					close();
				}}
				aria-label="No colour (use default)"
				aria-pressed={value === null}
				class="flex h-9 w-9 items-center justify-center rounded-full border-2 transition"
				class:border-fg={value === null}
				class:border-line={value !== null}
			>
				<span class="text-xs text-fg-faint">—</span>
			</button>
			{#each PROJECT_COLOUR_NAMES as c (c)}
				<button
					type="button"
					onclick={() => {
						value = c;
						close();
					}}
					aria-label={PROJECT_COLOUR_LABELS[c]}
					aria-pressed={value === c}
					title={PROJECT_COLOUR_LABELS[c]}
					class="h-9 w-9 rounded-full border-2 transition"
					class:border-fg={value === c}
					class:border-transparent={value !== c}
					style:background-color={projectColourVar(c)}
				></button>
			{/each}
		</div>
	{/snippet}
</Popover>
