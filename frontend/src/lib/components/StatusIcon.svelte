<script lang="ts">
	import type { ItemStatus } from '$lib/api/items';
	import { Circle, CircleDot, CheckCircle2, Archive } from '@lucide/svelte';

	type Props = { status: ItemStatus; class?: string };
	let { status, class: cls = 'h-4 w-4' }: Props = $props();
</script>

{#if status === 'open'}
	<Circle class="{cls} text-fg-faint" />
{:else if status === 'in_progress'}
	<CircleDot class="{cls} text-accent" />
{:else if status === 'done'}
	<CheckCircle2 class="{cls} text-success" />
{:else}
	<Archive class="{cls} text-fg-faint" />
{/if}
