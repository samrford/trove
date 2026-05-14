<script lang="ts">
	import { TAG_COLOUR_NAMES, TAG_COLOUR_LABELS, tagColourVar } from '$lib/tagColours';

	type Props = {
		value?: string | null;
		size?: 'sm' | 'md';
	};

	let { value = $bindable(null), size = 'md' }: Props = $props();

	const cls = $derived(size === 'sm' ? 'h-6 w-6' : 'h-9 w-9');
</script>

<div class="flex flex-wrap gap-2">
	<button
		type="button"
		onmousedown={(e) => e.preventDefault()}
		onclick={() => (value = null)}
		aria-label="No colour (default)"
		aria-pressed={value === null}
		class="flex items-center justify-center rounded-full border-2 transition {cls}"
		class:border-fg={value === null}
		class:border-line={value !== null}
	>
		<span class="text-xs text-fg-faint">—</span>
	</button>
	{#each TAG_COLOUR_NAMES as c (c)}
		<button
			type="button"
			onmousedown={(e) => e.preventDefault()}
			onclick={() => (value = c)}
			aria-label={TAG_COLOUR_LABELS[c]}
			aria-pressed={value === c}
			title={TAG_COLOUR_LABELS[c]}
			class="rounded-full border-2 transition {cls}"
			class:border-fg={value === c}
			class:border-transparent={value !== c}
			style:background-color={tagColourVar(c)}
		></button>
	{/each}
</div>
