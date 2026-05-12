<script lang="ts">
	import { animations } from '$lib/animations.svelte';
	import { MonitorStop, MonitorPlay } from '@lucide/svelte';
</script>

<button
	type="button"
	onclick={() => animations.toggle()}
	aria-label={animations.enabled ? 'Disable animations' : 'Enable animations'}
	aria-pressed={animations.enabled}
	title={animations.enabled ? 'Animations on — click to disable' : 'Animations off — click to enable'}
	class="rounded-md p-2 text-fg-muted transition hover:bg-card-2 hover:text-fg"
>
	{#if animations.enabled}
		<MonitorStop class="h-4 w-4" />
	{:else}
		<MonitorPlay class="h-4 w-4" />
	{/if}
</button>
