<script lang="ts">
	import './layout.css';
	import { auth } from '$lib/auth.svelte';
	import { sidebar } from '$lib/sidebar.svelte';
	import Sidebar from '$lib/components/Sidebar.svelte';

	let { children } = $props();
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
