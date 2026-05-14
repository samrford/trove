<script lang="ts">
	type Props = {
		open?: boolean;
		title: string;
		message: string;
		confirmLabel?: string;
		cancelLabel?: string;
		destructive?: boolean;
		onConfirm: () => void;
		onCancel?: () => void;
	};

	let {
		open = $bindable(false),
		title,
		message,
		confirmLabel = 'Confirm',
		cancelLabel = 'Cancel',
		destructive = false,
		onConfirm,
		onCancel
	}: Props = $props();

	let dialog = $state<HTMLDialogElement | undefined>(undefined);

	$effect(() => {
		if (!dialog) return;
		if (open && !dialog.open) dialog.showModal();
		else if (!open && dialog.open) dialog.close();
	});

	function handleCancel() {
		open = false;
		onCancel?.();
	}

	function handleConfirm() {
		open = false;
		onConfirm();
	}
</script>

<dialog
	bind:this={dialog}
	onclose={handleCancel}
	class="m-auto w-[calc(100%-1rem)] max-w-md rounded-lg border border-line bg-card p-4 text-fg shadow-xl backdrop:bg-black/50 backdrop:backdrop-blur-sm sm:p-6"
>
	<h2 class="text-lg font-semibold tracking-tight text-fg">{title}</h2>
	<p class="mt-2 text-sm text-fg-muted">{message}</p>
	<div class="mt-6 flex justify-end gap-2">
		<button
			type="button"
			onclick={handleCancel}
			class="rounded-md border border-line px-4 py-2 text-sm font-medium text-fg transition hover:bg-card-2"
		>
			{cancelLabel}
		</button>
		<button
			type="button"
			onclick={handleConfirm}
			class="rounded-md px-4 py-2 text-sm font-medium transition {destructive
				? 'bg-danger text-white hover:opacity-90'
				: 'bg-accent text-on-accent hover:bg-accent-hover'}"
		>
			{confirmLabel}
		</button>
	</div>
</dialog>
