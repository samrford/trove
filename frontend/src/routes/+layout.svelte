<script lang="ts">
	import './layout.css';

	let { children } = $props();
</script>

<svelte:head>
	<link rel="icon" type="image/png" href="/logosmall.png" />
	<link rel="apple-touch-icon" href="/logosmall.png" />
</svelte:head>

{@render children()}
