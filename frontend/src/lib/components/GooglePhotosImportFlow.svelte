<script lang="ts">
	import { createGooglePhotosFlow } from 'google-photos-picker-client/svelte';
	import type { FlowPhase } from 'google-photos-picker-client';
	import { apiFetch } from '$lib/api';
	import { onDestroy } from 'svelte';
	import { X, Loader2, ImagePlus, Check, RotateCcw } from '@lucide/svelte';

	// Single source for the flow phases so the literals aren't scattered.
	// `satisfies Record<FlowPhase, FlowPhase>` makes this fail to compile if the
	// library's phase set ever changes, instead of silently going stale.
	const Phase = {
		idle: 'idle',
		connecting: 'connecting',
		creating: 'creating',
		picking: 'picking',
		importing: 'importing',
		done: 'done',
		error: 'error'
	} as const satisfies Record<FlowPhase, FlowPhase>;

	type Props = {
		open?: boolean;
		slug: string;
		seq: number;
		// Fires after each newly-imported photo and on completion. The host
		// re-fetches the item so new attachments show up.
		onImported?: () => void;
	};

	let { open = $bindable(false), slug, seq, onImported }: Props = $props();

	// One flow for the component's life. Only `startImport` is item-scoped;
	// it's a function evaluated at call time, so it reads the *current* slug/seq
	const flow = createGooglePhotosFlow({
		postMessageType: 'trove:google-oauth',
		fetchJson: apiFetch,
		endpoints: {
			status: '/v1/google-photos/status',
			connect: '/v1/google-photos/connect',
			disconnect: '/v1/google-photos/disconnect',
			createSession: '/v1/google-photos/sessions',
			pollSession: (sid) => `/v1/google-photos/sessions/${encodeURIComponent(sid)}`,
			startImport: (sid) =>
				`/v1/projects/${encodeURIComponent(slug)}/items/${seq}/google-photos/sessions/${encodeURIComponent(sid)}/import`,
			getImport: (jobId) => `/v1/google-photos/imports/${encodeURIComponent(jobId)}`
		}
	});

	let wasOpen = $state(false);
	let lastImported = $state(0);

	// Kick a status probe when the dialog opens;
	// cancel any in-flight run when it closes.
	$effect(() => {
		if (open && !wasOpen) {
			wasOpen = true;
			lastImported = 0;
			flow.refreshStatus();
		} else if (!open && wasOpen) {
			wasOpen = false;
			flow.cancel();
		}
	});

	// Surface each newly-completed photo to the host (incremental appearance),
	// and guarantee a final refresh on done.
	$effect(() => {
		const done = $flow.progress?.completed ?? 0;
		if (done > lastImported) {
			lastImported = done;
			onImported?.();
		}
		if ($flow.phase === Phase.done) onImported?.();
	});

	onDestroy(() => flow.cancel());

	function close() {
		open = false;
	}

	const busy = $derived(
		$flow.phase === Phase.connecting ||
			$flow.phase === Phase.creating ||
			$flow.phase === Phase.picking ||
			$flow.phase === Phase.importing
	);

	const message = $derived.by(() => {
		switch ($flow.phase) {
			case Phase.connecting:
				return 'Authorise Google Photos in the popup…';
			case Phase.creating:
				return 'Opening Google Photos…';
			case Phase.picking:
				return 'Pick photos in the Google tab — we’ll grab them when you confirm.';
			case Phase.importing:
				return $flow.progress
					? `Importing… ${$flow.progress.completed}/${$flow.progress.total}`
					: 'Starting import…';
			case Phase.done:
				return $flow.result
					? `Imported ${$flow.result.completed} of ${$flow.result.total}.`
					: 'Imported.';
			default:
				return $flow.connected
					? 'Ready to import from Google Photos.'
					: 'Connect Google Photos to import.';
		}
	});

	const pct = $derived(
		$flow.progress && $flow.progress.total > 0
			? Math.round(($flow.progress.completed / $flow.progress.total) * 100)
			: 0
	);
</script>

{#if open}
	<div
		role="dialog"
		aria-modal="true"
		aria-labelledby="gphotos-title"
		class="fixed inset-0 z-[55] flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm"
	>
		<div class="w-full max-w-md rounded-lg border border-line bg-card p-5 shadow-2xl">
			<div class="mb-3 flex items-center justify-between">
				<h2
					id="gphotos-title"
					class="inline-flex items-center gap-2 text-base font-semibold text-fg"
				>
					<ImagePlus class="h-4 w-4 text-fg-muted" />
					Google Photos
				</h2>
				<button
					type="button"
					onclick={close}
					aria-label="Close"
					class="rounded-md p-1 text-fg-muted transition hover:bg-card-2 hover:text-fg"
				>
					<X class="h-4 w-4" />
				</button>
			</div>

			<div class="flex items-start gap-3">
				{#if $flow.phase === Phase.done}
					<Check class="mt-0.5 h-4 w-4 shrink-0 text-accent" />
				{:else if $flow.phase === Phase.error}
					<X class="mt-0.5 h-4 w-4 shrink-0 text-danger" />
				{:else if busy}
					<Loader2 class="mt-0.5 h-4 w-4 shrink-0 animate-spin text-fg-muted" />
				{:else}
					<ImagePlus class="mt-0.5 h-4 w-4 shrink-0 text-fg-muted" />
				{/if}
				<div class="min-w-0 flex-1">
					<p class="text-sm text-fg">{message}</p>
					{#if $flow.phase === Phase.error && $flow.error}
						<p class="mt-1 text-xs text-danger">{$flow.error}</p>
					{/if}
					{#if $flow.phase === Phase.importing && $flow.progress}
						<div class="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-card-2">
							<div class="h-full bg-accent transition-[width]" style:width="{pct}%"></div>
						</div>
					{/if}
				</div>
			</div>

			<div class="mt-5 flex justify-end gap-2">
				{#if $flow.phase === Phase.idle && !$flow.connected}
					<button
						type="button"
						onclick={() => flow.connect()}
						class="rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-on-accent transition hover:bg-accent-hover"
					>
						Connect Google Photos
					</button>
				{:else if $flow.phase === Phase.idle && $flow.connected}
					<button
						type="button"
						onclick={() => flow.start()}
						class="inline-flex items-center gap-1.5 rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-on-accent transition hover:bg-accent-hover"
					>
						<ImagePlus class="h-3.5 w-3.5" />
						Choose photos
					</button>
				{:else if $flow.phase === Phase.error}
					<button
						type="button"
						onclick={() => ($flow.connected ? flow.start() : flow.connect())}
						class="inline-flex items-center gap-1.5 rounded-md border border-line px-3 py-1.5 text-xs font-medium text-fg transition hover:bg-card-2"
					>
						<RotateCcw class="h-3.5 w-3.5" />
						{$flow.expired ? 'New session' : 'Retry'}
					</button>
				{/if}
				<button
					type="button"
					onclick={close}
					class="rounded-md border border-line px-3 py-1.5 text-xs font-medium text-fg transition hover:bg-card-2"
				>
					{$flow.phase === Phase.done || $flow.phase === Phase.error ? 'Close' : 'Cancel'}
				</button>
			</div>
		</div>
	</div>
{/if}
