<script lang="ts">
	import { createItem, type Item, type ItemKind } from '$lib/api/items';
	import { KIND_PLACEHOLDERS } from '$lib/itemDisplay';
	import KindPicker from './KindPicker.svelte';
	import { ApiError } from '$lib/api';

	type Props = {
		open?: boolean;
		projectSlug: string;
		onCreated: (item: Item) => void;
	};

	let { open = $bindable(false), projectSlug, onCreated }: Props = $props();

	let dialog = $state<HTMLDialogElement | undefined>(undefined);

	let kind = $state<ItemKind>('task');
	let title = $state('');
	let body = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	function reset() {
		kind = 'task';
		title = '';
		body = '';
		error = null;
		submitting = false;
	}

	$effect(() => {
		if (!dialog) return;
		if (open && !dialog.open) {
			reset();
			dialog.showModal();
			// Focus title after the dialog has rendered.
			requestAnimationFrame(() => {
				dialog?.querySelector<HTMLInputElement>('input[name="title"]')?.focus();
			});
		} else if (!open && dialog.open) {
			dialog.close();
		}
	});

	function handleCancel() {
		open = false;
	}

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		const trimmed = title.trim();
		if (!trimmed || submitting) return;
		submitting = true;
		error = null;
		try {
			const item = await createItem(projectSlug, {
				kind,
				title: trimmed,
				body: body.trim() || null
			});
			open = false;
			onCreated(item);
		} catch (e) {
			error = e instanceof ApiError ? e.message : String(e);
			submitting = false;
		}
	}
</script>

<dialog
	bind:this={dialog}
	onclose={handleCancel}
	class="m-auto w-full max-w-lg rounded-lg border border-line bg-card p-6 text-fg shadow-xl backdrop:bg-black/50 backdrop:backdrop-blur-sm"
>
	<form onsubmit={handleSubmit} class="flex flex-col gap-4">
		<h2 class="text-lg font-semibold tracking-tight text-fg">New item</h2>

		<div class="flex flex-col gap-2">
			<span class="text-sm font-medium text-fg">Kind</span>
			<KindPicker value={kind} onSelect={(k) => (kind = k)} />
		</div>

		<label class="flex flex-col gap-2">
			<span class="text-sm font-medium text-fg">Title</span>
			<input
				type="text"
				name="title"
				bind:value={title}
				required
				maxlength={200}
				placeholder={KIND_PLACEHOLDERS[kind]}
				class="rounded-md border border-line bg-card px-3 py-2 text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
			/>
		</label>

		<label class="flex flex-col gap-2">
			<span class="text-sm font-medium text-fg"
				>Notes <span class="font-normal text-fg-faint">(optional, markdown)</span></span
			>
			<textarea
				bind:value={body}
				rows="4"
				placeholder="Anything more to say? Markdown welcome."
				class="resize-none rounded-md border border-line bg-card px-3 py-2 font-mono text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
			></textarea>
		</label>

		{#if error}
			<p class="text-sm text-danger">{error}</p>
		{/if}

		<div class="mt-2 flex justify-end gap-2">
			<button
				type="button"
				onclick={handleCancel}
				class="rounded-md border border-line px-4 py-2 text-sm font-medium text-fg transition hover:bg-card-2"
			>
				Cancel
			</button>
			<button
				type="submit"
				disabled={submitting || !title.trim()}
				class="rounded-md bg-accent px-4 py-2 text-sm font-medium text-on-accent transition hover:bg-accent-hover disabled:opacity-50"
			>
				{submitting ? 'Creating…' : 'Create item'}
			</button>
		</div>
	</form>
</dialog>
