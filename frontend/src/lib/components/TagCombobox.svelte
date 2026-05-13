<script lang="ts">
	import type { Tag, TagWithCount } from '$lib/api/tags';
	import TagChip from './TagChip.svelte';
	import EmojiPicker from './EmojiPicker.svelte';
	import TagColourPicker from './TagColourPicker.svelte';
	import { Plus } from '@lucide/svelte';
	import { tagColourFromName, tagColourVar } from '$lib/tagColours';
	import { slugify } from '$lib/slug';

	type Props = {
		selected: Tag[];
		availableTags: TagWithCount[];
		// For new tags, the parent may receive a draft with colour/icon set.
		onAdd: (
			tag: Tag | { name: string; colour?: string | null; icon?: string | null }
		) => void | Promise<void>;
		onRemove: (tag: Tag) => void | Promise<void>;
		placeholder?: string;
	};

	let { selected, availableTags, onAdd, onRemove, placeholder = 'Add a tag…' }: Props = $props();

	let query = $state('');
	let inputEl: HTMLInputElement | undefined = $state();
	let dropdownEl: HTMLDivElement | undefined = $state();
	let dropdownOpen = $state(false);
	let highlightedIndex = $state(0);

	// Inline "new tag" configurator state — when the user is about to create.
	let configuringNew = $state(false);
	let newColour = $state<string | null>(null);
	let newIcon = $state('');

	const selectedSlugs = $derived(new Set(selected.map((t) => t.slug)));

	// Filter available tags by query (case-insensitive substring on name), and
	// exclude those already selected.
	const filtered = $derived.by<TagWithCount[]>(() => {
		const q = query.trim().toLowerCase();
		const pool = availableTags.filter((t) => !selectedSlugs.has(t.slug));
		if (!q) {
			// No query — sort top N (5) by last_used_at DESC, then rest alpha.
			const recent = pool
				.filter((t) => t.last_used_at)
				.sort((a, b) => (a.last_used_at! < b.last_used_at! ? 1 : -1))
				.slice(0, 5);
			const recentSlugs = new Set(recent.map((t) => t.slug));
			const rest = pool
				.filter((t) => !recentSlugs.has(t.slug))
				.sort((a, b) => a.name.localeCompare(b.name));
			return [...recent, ...rest];
		}
		return pool
			.filter((t) => t.name.toLowerCase().includes(q))
			.sort((a, b) => a.name.localeCompare(b.name));
	});

	// Should we offer "Create 'foo'"? Only when query has content AND no
	// case-insensitive match in the full pool exists.
	const exactMatchExists = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (!q) return false;
		return availableTags.some((t) => t.name.toLowerCase() === q);
	});
	const canCreate = $derived.by(() => {
		return query.trim() !== '' && !exactMatchExists;
	});

	// Length used by keyboard navigation: filtered count + (1 if canCreate).
	const optionCount = $derived(filtered.length + (canCreate ? 1 : 0));

	$effect(() => {
		void query;
		highlightedIndex = 0;
	});

	async function pickExisting(tag: TagWithCount) {
		await onAdd(tag);
		query = '';
		highlightedIndex = 0;
		configuringNew = false;
		inputEl?.focus();
	}

	function startConfiguringNew() {
		if (!canCreate) return;
		newColour = tagColourFromName(query.trim());
		newIcon = '';
		configuringNew = true;
	}

	async function commitNew() {
		const trimmed = query.trim();
		if (!trimmed) return;
		await onAdd({ name: trimmed, colour: newColour, icon: newIcon.trim() || null });
		query = '';
		highlightedIndex = 0;
		configuringNew = false;
		newColour = null;
		newIcon = '';
		inputEl?.focus();
	}

	function cancelConfiguringNew() {
		configuringNew = false;
		newColour = null;
		newIcon = '';
		inputEl?.focus();
	}

	async function applyHighlight() {
		if (highlightedIndex < filtered.length) {
			await pickExisting(filtered[highlightedIndex]);
		} else if (canCreate) {
			startConfiguringNew();
		}
	}

	function onKeyDown(e: KeyboardEvent) {
		if (!dropdownOpen) return;
		if (configuringNew) {
			if (e.key === 'Escape') {
				e.preventDefault();
				cancelConfiguringNew();
			} else if (e.key === 'Enter') {
				e.preventDefault();
				commitNew();
			}
			return;
		}
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			highlightedIndex = Math.min(highlightedIndex + 1, Math.max(optionCount - 1, 0));
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			highlightedIndex = Math.max(highlightedIndex - 1, 0);
		} else if (e.key === 'Enter') {
			if (optionCount === 0) return;
			e.preventDefault();
			applyHighlight();
		} else if (e.key === 'Escape') {
			query = '';
			dropdownOpen = false;
			inputEl?.blur();
		} else if (e.key === 'Backspace' && query === '' && selected.length > 0) {
			// Quick remove the last chip with backspace on empty input.
			e.preventDefault();
			onRemove(selected[selected.length - 1]);
		}
	}

	function handleFocus() {
		dropdownOpen = true;
	}

	function handleBlur(e: FocusEvent) {
		// Only close if focus actually left the combobox (input + dropdown).
		// Without this check, clicks inside the dropdown blur the input and
		// race the click handler, making picks finicky.
		const next = e.relatedTarget as Node | null;
		if (next && (dropdownEl?.contains(next) || inputEl?.contains(next))) return;
		dropdownOpen = false;
		configuringNew = false;
	}

	// Draft tag for live preview while configuring a new one.
	const newPreviewTag = $derived<Tag>({
		id: 'preview',
		slug: slugify(query.trim()) || 'tag',
		name: query.trim() || 'tag',
		description: null,
		icon: newIcon.trim() || null,
		colour: newColour,
		user_id: null,
		group_id: null,
		archived_at: null,
		created_at: new Date().toISOString(),
		updated_at: new Date().toISOString()
	});
</script>

<div class="relative">
	<div
		class="flex flex-wrap items-center gap-1.5 rounded-md border border-line bg-card px-2 py-1.5 focus-within:border-accent"
	>
		{#each selected as tag (tag.id)}
			<TagChip {tag} size="sm" onRemove={() => onRemove(tag)} />
		{/each}
		<input
			bind:this={inputEl}
			type="text"
			bind:value={query}
			onfocus={handleFocus}
			onblur={handleBlur}
			onkeydown={onKeyDown}
			{placeholder}
			autocomplete="off"
			autocapitalize="off"
			spellcheck={false}
			class="min-w-32 flex-1 bg-transparent py-0.5 text-sm text-fg placeholder:text-fg-faint focus:outline-none"
		/>
	</div>

	{#if dropdownOpen && (filtered.length > 0 || canCreate)}
		<div
			bind:this={dropdownEl}
			role="listbox"
			class="absolute top-full right-0 left-0 z-20 mt-1 max-h-72 overflow-auto rounded-md border border-line bg-card shadow-lg"
		>
			{#if configuringNew}
				<!-- Inline new-tag configurator: chip preview + colour + icon + create. -->
				<div class="flex flex-col gap-3 p-3">
					<div class="flex items-center justify-between">
						<span class="text-xs font-medium tracking-wide text-fg-faint uppercase">New tag</span>
						<TagChip tag={newPreviewTag} size="sm" />
					</div>

					<div class="flex flex-col gap-1.5">
						<span class="text-xs text-fg-muted">Colour</span>
						<TagColourPicker bind:value={newColour} size="sm" />
					</div>

					<div class="flex flex-col gap-1.5">
						<span class="text-xs text-fg-muted">Icon</span>
						<EmojiPicker bind:value={newIcon} />
					</div>

					<div class="flex items-center justify-end gap-2">
						<button
							type="button"
							onmousedown={(e) => e.preventDefault()}
							onclick={cancelConfiguringNew}
							class="rounded-md px-2.5 py-1 text-xs text-fg-muted transition hover:bg-card-2 hover:text-fg"
						>
							Cancel
						</button>
						<button
							type="button"
							onmousedown={(e) => e.preventDefault()}
							onclick={commitNew}
							class="rounded-md bg-accent px-2.5 py-1 text-xs font-medium text-on-accent transition hover:bg-accent-hover"
						>
							Create
						</button>
					</div>
				</div>
			{:else}
				{#each filtered as tag, i (tag.id)}
					<button
						type="button"
						role="option"
						aria-selected={highlightedIndex === i}
						onmousedown={(e) => e.preventDefault()}
						onclick={() => pickExisting(tag)}
						onmouseenter={() => (highlightedIndex = i)}
						class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm transition"
						class:bg-card-2={highlightedIndex === i}
					>
						<span class="h-2.5 w-2.5 rounded-full" style:background-color={tagColourVar(tag.colour)}
						></span>
						{#if tag.icon}<span aria-hidden="true">{tag.icon}</span>{/if}
						<span class="flex-1 truncate text-fg">{tag.name}</span>
						{#if tag.item_count > 0}
							<span class="text-xs text-fg-faint">{tag.item_count}</span>
						{/if}
					</button>
				{/each}

				{#if canCreate}
					{@const createIndex = filtered.length}
					<button
						type="button"
						role="option"
						aria-selected={highlightedIndex === createIndex}
						onmousedown={(e) => e.preventDefault()}
						onclick={startConfiguringNew}
						onmouseenter={() => (highlightedIndex = createIndex)}
						class="flex w-full items-center gap-2 border-t border-line px-3 py-1.5 text-left text-sm transition"
						class:bg-card-2={highlightedIndex === createIndex}
					>
						<Plus class="h-3.5 w-3.5 text-fg-muted" />
						<span class="text-fg">Create &quot;{query.trim()}&quot;…</span>
					</button>
				{/if}
			{/if}
		</div>
	{/if}
</div>
