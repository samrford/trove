<script lang="ts">
	import type { Snippet } from 'svelte';

	type TriggerArgs = { open: boolean; toggle: () => void };
	type ContentArgs = { close: () => void };

	type Props = {
		label: string;
		trigger: Snippet<[TriggerArgs]>;
		children: Snippet<[ContentArgs]>;
	};

	let { label, trigger, children }: Props = $props();

	let open = $state(false);
	let wrapperEl: HTMLDivElement | undefined = $state();
	let popoverEl: HTMLDivElement | undefined = $state();

	function toggle() {
		open = !open;
	}

	function close() {
		open = false;
		wrapperEl?.querySelector<HTMLButtonElement>('button')?.focus();
	}

	$effect(() => {
		if (!open) return;
		function onKey(e: KeyboardEvent) {
			if (e.key === 'Escape') close();
		}
		function onPointer(e: MouseEvent) {
			const target = e.target as Node;
			if (popoverEl?.contains(target) || wrapperEl?.contains(target)) return;
			open = false;
		}
		document.addEventListener('keydown', onKey);
		document.addEventListener('mousedown', onPointer);
		return () => {
			document.removeEventListener('keydown', onKey);
			document.removeEventListener('mousedown', onPointer);
		};
	});
</script>

<div bind:this={wrapperEl} class="relative inline-block">
	{@render trigger({ open, toggle })}

	{#if open}
		<div
			bind:this={popoverEl}
			role="dialog"
			aria-label={label}
			class="absolute left-0 top-full z-20 mt-2 rounded-md border border-line bg-card p-2 shadow-lg"
		>
			{@render children({ close })}
		</div>
	{/if}
</div>
