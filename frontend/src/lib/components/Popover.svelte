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
	let coords = $state<{ top: number; left: number }>({ top: 0, left: 0 });

	function recomputeCoords() {
		if (!wrapperEl) return;
		const rect = wrapperEl.getBoundingClientRect();
		coords = { top: rect.bottom + 6, left: rect.left };
	}

	function toggle() {
		if (!open) recomputeCoords();
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
		function onReposition() {
			recomputeCoords();
		}
		document.addEventListener('keydown', onKey);
		document.addEventListener('mousedown', onPointer);
		window.addEventListener('scroll', onReposition, true);
		window.addEventListener('resize', onReposition);
		return () => {
			document.removeEventListener('keydown', onKey);
			document.removeEventListener('mousedown', onPointer);
			window.removeEventListener('scroll', onReposition, true);
			window.removeEventListener('resize', onReposition);
		};
	});
</script>

<div bind:this={wrapperEl} class="relative inline-block">
	{@render trigger({ open, toggle })}
</div>

{#if open}
	<div
		bind:this={popoverEl}
		role="dialog"
		aria-label={label}
		style:top="{coords.top}px"
		style:left="{coords.left}px"
		class="fixed z-50 rounded-md border border-line bg-card p-2 shadow-lg"
	>
		{@render children({ close })}
	</div>
{/if}
