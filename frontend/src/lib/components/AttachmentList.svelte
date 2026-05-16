<script lang="ts">
	import type { Attachment } from '$lib/api/items';
	import { deleteAttachment } from '$lib/api/attachments';
	import { errMsg } from '$lib/api';
	import { SvelteSet } from 'svelte/reactivity';
	import { Download, Trash2, FileText, X, Image as ImageIcon } from '@lucide/svelte';

	type Props = {
		slug: string;
		seq: number;
		attachments: Attachment[];
		onDeleted?: (attachment: Attachment) => void;
		readOnly?: boolean;
	};

	let { slug, seq, attachments, onDeleted, readOnly = false }: Props = $props();

	let actionError = $state<string | null>(null);
	let lightboxUrl = $state<string | null>(null);
	const deleting = new SvelteSet<string>();

	function isImage(a: Attachment): boolean {
		return a.content_type.startsWith('image/');
	}

	function humanSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
	}

	async function handleDelete(a: Attachment) {
		if (deleting.has(a.id)) return;
		deleting.add(a.id);
		try {
			await deleteAttachment(slug, seq, a.id);
			onDeleted?.(a);
		} catch (e) {
			actionError = errMsg(e);
		} finally {
			deleting.delete(a.id);
		}
	}
</script>

{#if actionError}
	<div
		class="mb-2 flex items-start gap-2 rounded-md border border-danger/40 bg-danger/10 px-2 py-1.5 text-xs text-danger"
	>
		<span class="flex-1">{actionError}</span>
		<button
			type="button"
			onclick={() => (actionError = null)}
			aria-label="Dismiss"
			class="text-danger/70 transition hover:text-danger"
		>
			<X class="h-3.5 w-3.5" />
		</button>
	</div>
{/if}

{#if attachments.length === 0}
	<p class="text-xs text-fg-faint italic">No attachments yet.</p>
{:else}
	<ul class="flex flex-col gap-2">
		{#each attachments as a (a.id)}
			<li class="group flex items-center gap-3 rounded-md border border-line bg-card-2/40 p-2">
				{#if isImage(a)}
					<button
						type="button"
						onclick={() => (lightboxUrl = a.url)}
						class="block h-14 w-14 shrink-0 overflow-hidden rounded-md bg-card transition hover:opacity-80"
						aria-label={`Open ${a.filename}`}
					>
						<img src={a.url} alt={a.filename} class="h-full w-full object-cover" loading="lazy" />
					</button>
				{:else}
					<div
						class="flex h-14 w-14 shrink-0 items-center justify-center rounded-md bg-card text-fg-muted"
						aria-hidden="true"
					>
						<FileText class="h-6 w-6" />
					</div>
				{/if}
				<div class="min-w-0 flex-1">
					<p class="truncate text-sm text-fg" title={a.filename}>{a.filename}</p>
					<p class="text-xs text-fg-faint">
						{humanSize(a.size_bytes)} ·
						{a.source === 'google_photos' ? 'Google Photos' : 'Upload'}
					</p>
				</div>
				<a
					href={a.url}
					download={a.filename}
					target="_blank"
					rel="noopener"
					aria-label="Download"
					title="Download"
					class="rounded-md p-1.5 text-fg-muted transition hover:bg-card-2 hover:text-fg"
				>
					<Download class="h-4 w-4" />
				</a>
				{#if !readOnly}
					<button
						type="button"
						onclick={() => handleDelete(a)}
						disabled={deleting.has(a.id)}
						aria-label="Delete attachment"
						title="Delete"
						class="rounded-md p-1.5 text-fg-muted transition hover:bg-danger/10 hover:text-danger disabled:opacity-40"
					>
						<Trash2 class="h-4 w-4" />
					</button>
				{/if}
			</li>
		{/each}
	</ul>
{/if}

{#if lightboxUrl}
	<button
		type="button"
		aria-label="Close preview"
		onclick={() => (lightboxUrl = null)}
		class="fixed inset-0 z-[60] flex items-center justify-center bg-black/80 p-4"
	>
		<img
			src={lightboxUrl}
			alt="Attachment preview"
			class="max-h-full max-w-full rounded-lg shadow-2xl"
		/>
		<span
			class="absolute top-4 right-4 inline-flex items-center gap-1 rounded-md bg-card/80 px-2 py-1 text-xs text-fg-muted backdrop-blur"
		>
			<ImageIcon class="h-3.5 w-3.5" />
			click anywhere to close
		</span>
	</button>
{/if}
