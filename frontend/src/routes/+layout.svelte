<script lang="ts">
	import './layout.css';
	import { auth } from '$lib/auth.svelte';
	import { sidebar } from '$lib/sidebar.svelte';
	import { appConfig } from '$lib/config.svelte';
	import { realtime } from '$lib/realtime.svelte';
	import Sidebar from '$lib/components/Sidebar.svelte';

	let { children } = $props();

	$effect(() => {
		if (auth.user && !appConfig.config && !appConfig.loading) {
			appConfig.load();
		}
	});

	// One long-lived SSE connection per signed-in user; teardown on sign-out.
	// Token rotation is handled internally by the store.
	$effect(() => {
		if (auth.user) realtime.start();
		else realtime.stop();
		return () => realtime.stop();
	});
</script>

<svelte:head>
	<link rel="icon" type="image/png" href="/logosmall.png" />
	<link rel="apple-touch-icon" href="/logosmall.png" />
</svelte:head>

{#if auth.user}
	<Sidebar />
	<div
		class="transition-[padding] duration-200"
		class:lg:pl-60={!sidebar.collapsed}
		class:lg:pl-16={sidebar.collapsed}
	>
		{@render children()}
	</div>
{:else}
	{@render children()}
{/if}
