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
	// Hide the popover for one frame so we can measure it and apply
	// clamping/flipping before it appears — avoids a flash off-screen on mobile.
	let measured = $state(false);

	function recomputeCoords() {
		if (!wrapperEl) return;
		const rect = wrapperEl.getBoundingClientRect();
		const margin = 8;
		// Once popoverEl is mounted we know its size — clamp horizontally so it
		// can't fall off the viewport on narrow screens.
		const width = popoverEl?.offsetWidth ?? 0;
		const height = popoverEl?.offsetHeight ?? 0;
		const vw = window.innerWidth;
		const vh = window.innerHeight;

		let left = rect.left;
		if (width > 0) {
			left = Math.min(left, vw - width - margin);
			left = Math.max(margin, left);
		}

		let top = rect.bottom + 6;
		// Flip above the trigger if there's not enough room below.
		if (height > 0 && top + height + margin > vh && rect.top - height - 6 >= margin) {
			top = rect.top - height - 6;
		}

		coords = { top, left };
	}

	function toggle() {
		if (!open) {
			measured = false;
			recomputeCoords();
		}
		open = !open;
	}

	function close() {
		open = false;
		measured = false;
		wrapperEl?.querySelector<HTMLButtonElement>('button')?.focus();
	}

	// Once the popover element mounts, re-measure with its true size.
	$effect(() => {
		if (open && popoverEl && !measured) {
			recomputeCoords();
			measured = true;
		}
	});

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
		class:invisible={!measured}
	>
		{@render children({ close })}
	</div>
{/if}
