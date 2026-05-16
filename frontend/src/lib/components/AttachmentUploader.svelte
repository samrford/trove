<script lang="ts">
	import type { Attachment } from '$lib/api/items';
	import { uploadAttachment, type UploadProgress } from '$lib/api/attachments';
	import { appConfig } from '$lib/config.svelte';
	import { errMsg } from '$lib/api';
	import { Upload, X, Loader2, ImagePlus } from '@lucide/svelte';

	type Props = {
		slug: string;
		seq: number;
		onUploaded?: (attachment: Attachment) => void;
		onGooglePhotosClick?: () => void;
	};

	let { slug, seq, onUploaded, onGooglePhotosClick }: Props = $props();

	type Tracker = {
		id: string;
		file: File;
		loaded: number;
		total: number;
		error: string | null;
		done: boolean;
	};

	let trackers = $state<Tracker[]>([]);
	let dragActive = $state(false);
	let inputEl: HTMLInputElement | undefined = $state();

	const maxBytes = $derived(appConfig.config?.maxAttachmentBytes ?? 25 * 1024 * 1024);
	const maxMB = $derived(Math.floor(maxBytes / (1024 * 1024)));
	const googleEnabled = $derived(appConfig.config?.googlePhotosEnabled ?? false);

	function nextID(): string {
		return Math.random().toString(36).slice(2);
	}

	async function startUploads(files: File[]) {
		// Filter for the size cap up front so the user gets immediate feedback
		// before the server says 413.
		const valid: File[] = [];
		const invalid: Tracker[] = [];
		for (const f of files) {
			if (f.size > maxBytes) {
				invalid.push({
					id: nextID(),
					file: f,
					loaded: 0,
					total: f.size,
					error: `File exceeds ${maxMB}MB cap`,
					done: false
				});
			} else {
				valid.push(f);
			}
		}
		const newTrackers = valid.map<Tracker>((f) => ({
			id: nextID(),
			file: f,
			loaded: 0,
			total: f.size,
			error: null,
			done: false
		}));
		trackers = [...trackers, ...newTrackers, ...invalid];

		await Promise.allSettled(
			newTrackers.map(async (t) => {
				try {
					const attachment = await uploadAttachment(slug, seq, t.file, (p: UploadProgress) => {
						trackers = trackers.map((x) =>
							x.id === t.id ? { ...x, loaded: p.loaded, total: p.total || x.total } : x
						);
					});
					trackers = trackers.map((x) => (x.id === t.id ? { ...x, done: true } : x));
					onUploaded?.(attachment);
					// Auto-clear successful trackers after a short delay so the UI
					// doesn't accumulate finished rows.
					setTimeout(() => {
						trackers = trackers.filter((x) => x.id !== t.id);
					}, 1500);
				} catch (e) {
					const message =
						errMsg(e);
					trackers = trackers.map((x) => (x.id === t.id ? { ...x, error: message } : x));
				}
			})
		);
	}

	function handleFiles(fl: FileList | null) {
		if (!fl || fl.length === 0) return;
		startUploads(Array.from(fl));
	}

	function handleDragOver(e: DragEvent) {
		e.preventDefault();
		dragActive = true;
	}
	function handleDragLeave(e: DragEvent) {
		if (e.currentTarget !== e.target) return;
		dragActive = false;
	}
	function handleDrop(e: DragEvent) {
		e.preventDefault();
		dragActive = false;
		handleFiles(e.dataTransfer?.files ?? null);
	}

	function dismissTracker(id: string) {
		trackers = trackers.filter((t) => t.id !== id);
	}

	function pct(t: Tracker): number {
		if (t.done) return 100;
		if (!t.total) return 0;
		return Math.min(100, Math.round((t.loaded / t.total) * 100));
	}
</script>

<div
	role="region"
	aria-label="Upload attachments"
	ondragover={handleDragOver}
	ondragleave={handleDragLeave}
	ondrop={handleDrop}
	class="flex flex-col gap-2 rounded-md border border-dashed p-3 transition {dragActive
		? 'border-accent bg-accent/10'
		: 'border-line bg-card-2/30'}"
>
	<div class="flex flex-wrap items-center gap-2">
		<button
			type="button"
			onclick={() => inputEl?.click()}
			class="inline-flex items-center gap-1.5 rounded-md border border-line bg-card px-3 py-1.5 text-xs font-medium text-fg transition hover:bg-card-2"
		>
			<Upload class="h-3.5 w-3.5" />
			Choose files
		</button>
		{#if googleEnabled && onGooglePhotosClick}
			<button
				type="button"
				onclick={onGooglePhotosClick}
				class="inline-flex items-center gap-1.5 rounded-md border border-line bg-card px-3 py-1.5 text-xs font-medium text-fg transition hover:bg-card-2"
			>
				<ImagePlus class="h-3.5 w-3.5" />
				Import from Google Photos
			</button>
		{/if}
		<span class="text-xs text-fg-faint">
			{dragActive ? 'Drop to upload' : `Drag files here or pick from disk · max ${maxMB}MB each`}
		</span>
	</div>

	<input
		bind:this={inputEl}
		type="file"
		multiple
		hidden
		onchange={(e) => {
			const target = e.currentTarget;
			handleFiles(target.files);
			target.value = '';
		}}
	/>

	{#if trackers.length > 0}
		<ul class="flex flex-col gap-1.5">
			{#each trackers as t (t.id)}
				<li
					class="flex items-center gap-2 rounded-md border border-line bg-card px-2 py-1.5 text-xs"
				>
					{#if t.error}
						<X class="h-3.5 w-3.5 shrink-0 text-danger" />
					{:else if t.done}
						<span class="h-3.5 w-3.5 shrink-0 rounded-full bg-accent" aria-hidden="true"></span>
					{:else}
						<Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin text-fg-muted" />
					{/if}
					<span class="min-w-0 flex-1 truncate text-fg" title={t.file.name}>{t.file.name}</span>
					{#if t.error}
						<span class="text-danger">{t.error}</span>
					{:else}
						<span class="text-fg-faint">{pct(t)}%</span>
						<div class="h-1 w-16 overflow-hidden rounded-full bg-card-2">
							<div class="h-full bg-accent transition-[width]" style:width="{pct(t)}%"></div>
						</div>
					{/if}
					<button
						type="button"
						onclick={() => dismissTracker(t.id)}
						aria-label="Dismiss"
						class="rounded p-0.5 text-fg-muted transition hover:bg-card-2 hover:text-fg"
					>
						<X class="h-3 w-3" />
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>
