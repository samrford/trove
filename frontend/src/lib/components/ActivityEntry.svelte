<script lang="ts">
	import StatusIcon from './StatusIcon.svelte';
	import { relativeTime } from '$lib/time';
	import { STATUS_LABEL, KIND_LABEL } from '$lib/itemDisplay';
	import { describeActivity, type Activity, type ActivityIcon } from '$lib/activity';
	import {
		Plus,
		Pencil,
		Trash2,
		Paperclip,
		Tag,
		ArrowUpDown,
		Folder,
		MessageSquare,
		ArrowRight
	} from '@lucide/svelte';

	// One component, three densities (locked design): `rail` = per-item
	// timeline, `compact` = slide-in panel, `rich` = the dedicated page.
	// `showItemRef` adds the `#seq` (project surfaces; off on the item rail).
	// All wording/classification comes from describeActivity — this file is
	// presentation only.

	type Density = 'rail' | 'compact' | 'rich';
	type Props = { entry: Activity; density?: Density; showItemRef?: boolean };
	let { entry, density = 'compact', showItemRef = false }: Props = $props();

	const d = $derived(describeActivity(entry));
	const when = $derived(relativeTime(entry.created_at));
	const absolute = $derived(new Date(entry.created_at).toLocaleString());

	const ICONS: Record<Exclude<ActivityIcon, 'status'>, typeof Plus> = {
		created: Plus,
		edited: Pencil,
		kind: Tag,
		tag: Tag,
		untag: Tag,
		attachment: Paperclip,
		deleted: Trash2,
		project: Folder,
		reorder: ArrowUpDown,
		note: MessageSquare
	};
	const Icon = $derived(d.icon === 'status' ? null : ICONS[d.icon]);
</script>

<div class="flex items-start gap-2.5 text-sm" data-density={density}>
	{#if density === 'rail'}
		<span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-line" aria-hidden="true"></span>
	{/if}

	<span class="mt-0.5 shrink-0 text-fg-faint">
		{#if d.icon === 'status' && d.statusChange}
			<StatusIcon status={d.statusChange.to} class="h-4 w-4" />
		{:else if Icon}
			<Icon class="h-4 w-4" />
		{/if}
	</span>

	<div class="min-w-0 flex-1">
		<div class="flex flex-wrap items-center gap-x-1.5 gap-y-0.5">
			{#if showItemRef && d.itemRef}
				<span class="font-mono text-xs text-fg-faint" data-testid="item-ref">
					#{d.itemRef.seq}
				</span>
			{/if}
			<span class="text-fg">
				<span class="font-medium">You</span>
				{d.verb}
			</span>

			{#if d.statusChange}
				<span class="inline-flex items-center gap-1 text-fg-muted">
					<StatusIcon status={d.statusChange.from} class="h-3.5 w-3.5" />
					{STATUS_LABEL[d.statusChange.from]}
					<ArrowRight class="h-3 w-3 text-fg-faint" />
					<StatusIcon status={d.statusChange.to} class="h-3.5 w-3.5" />
					{STATUS_LABEL[d.statusChange.to]}
				</span>
			{:else if d.kindChange}
				<span class="text-fg-muted">
					{KIND_LABEL[d.kindChange.from]} → {KIND_LABEL[d.kindChange.to]}
				</span>
			{:else if d.detail}
				<span class="truncate text-fg-muted">{d.detail}</span>
			{/if}

			{#if showItemRef && density === 'rich' && d.itemRef}
				<span class="truncate text-xs text-fg-faint">· {d.itemRef.title}</span>
			{/if}
		</div>

		{#if d.subLines?.length}
			<ul class="mt-0.5 space-y-0.5">
				{#each d.subLines as line, i (i)}
					<li class="text-xs text-fg-muted">{line}</li>
				{/each}
			</ul>
		{/if}
	</div>

	<time datetime={entry.created_at} title={absolute} class="mt-0.5 shrink-0 text-xs text-fg-faint">
		{when}
	</time>
</div>
