<script lang="ts">
	import { slugify, isValidSlug } from '$lib/slug';
	import { checkSlug } from '$lib/api/projects';
	import { Check, X, Loader2 } from '@lucide/svelte';

	export type SlugStatus = 'idle' | 'invalid' | 'checking' | 'available' | 'taken';

	type Props = {
		value?: string;
		status?: SlugStatus;
		name: string;
		// undefined = create form; defined (even if "") = edit form.
		originalName?: string;
		originalSlug?: string;
		id?: string;
	};

	let {
		value = $bindable(''),
		status = $bindable<SlugStatus>('idle'),
		name,
		originalName,
		originalSlug,
		id
	}: Props = $props();

	const isCreating = $derived(originalSlug === undefined);
	const suggestion = $derived(slugify(name));
	const showMismatch = $derived(
		value !== '' &&
			suggestion !== '' &&
			value !== suggestion &&
			(isCreating || originalName !== name)
	);
	const placeholderText = $derived(
		isCreating && suggestion
			? `Enter here, otherwise it will be set to ${suggestion}`
			: suggestion || 'project-slug'
	);

	let abortCtrl: AbortController | undefined;
	let debounceTimer: ReturnType<typeof setTimeout> | undefined;

	$effect(() => {
		clearTimeout(debounceTimer);
		abortCtrl?.abort();

		const trimmed = value.trim();

		if (!trimmed) {
			status = 'idle';
			return;
		}
		if (!isValidSlug(trimmed)) {
			status = 'invalid';
			return;
		}
		if (originalSlug !== undefined && trimmed === originalSlug) {
			status = 'idle';
			return;
		}

		status = 'checking';
		const ctrl = new AbortController();
		abortCtrl = ctrl;
		debounceTimer = setTimeout(async () => {
			try {
				const res = await checkSlug(trimmed, ctrl.signal);
				if (ctrl.signal.aborted) return;
				if (res.reason === 'invalid') status = 'invalid';
				else status = res.available ? 'available' : 'taken';
			} catch (e) {
				if ((e as { name?: string })?.name === 'AbortError') return;
				status = 'idle';
			}
		}, 350);
	});
</script>

<div class="flex flex-col gap-1.5">
	<div class="flex items-center gap-3">
		<input
			{id}
			type="text"
			bind:value
			placeholder={placeholderText}
			maxlength={120}
			autocomplete="off"
			autocapitalize="off"
			spellcheck={false}
			class="flex-1 rounded-md border border-line bg-card px-3 py-2 font-mono text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
		/>

		{#if status === 'checking'}
			<span class="flex items-center gap-1 text-xs text-fg-faint">
				<Loader2 class="h-3.5 w-3.5 animate-spin" />
				Checking…
			</span>
		{:else if status === 'available'}
			<span class="flex items-center gap-1 text-xs text-success">
				<Check class="h-3.5 w-3.5" />
				Available
			</span>
		{:else if status === 'taken'}
			<span class="flex items-center gap-1 text-xs text-danger">
				<X class="h-3.5 w-3.5" />
				Already taken
			</span>
		{:else if status === 'invalid'}
			<span class="flex items-center gap-1 text-xs text-danger">
				<X class="h-3.5 w-3.5" />
				Invalid format
			</span>
		{/if}
	</div>

	{#if showMismatch}
		<p class="text-xs text-danger">
			The slug won't match your project name. Leave it as is, or update it — perhaps to
			<button
				type="button"
				onclick={() => (value = suggestion)}
				class="font-mono font-medium underline underline-offset-2 hover:opacity-80"
				>{suggestion}</button
			>?
		</p>
	{/if}
</div>
