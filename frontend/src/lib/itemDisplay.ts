// Shared display metadata for items — labels, palette mappings, and chip
// styling. Keep this the single source of truth so adding a new kind/status
// is a one-place change.

import type { ItemKind, ItemStatus } from '$lib/api/items';

export const KIND_LABEL: Record<ItemKind, string> = {
	brainstorm: 'Brainstorm',
	task: 'Task'
};

export const KIND_PLURAL: Record<ItemKind, string> = {
	brainstorm: 'Brainstorms',
	task: 'Tasks'
};

// Kind chip colours pulled from the project palette so they live in the same
// world as the project accents.
export const KIND_COLOUR: Record<ItemKind, string> = {
	brainstorm: 'plum',
	task: 'gold'
};

export const STATUS_LABEL: Record<ItemStatus, string> = {
	open: 'Open',
	in_progress: 'In progress',
	done: 'Done',
	archived: 'Archived'
};

export const KIND_PLACEHOLDERS: Record<ItemKind, string> = {
	brainstorm: 'A loose idea, a half-thought, a spark…',
	task: "What's the task?"
};

// Inline-style CSS for a kind chip. Pass `active=false` to render the muted
// "not selected" variant used in toggle button groups (kind picker, etc.).
// The `border-color` is harmless on display-only chips (no border-width set).
export function kindChipStyle(k: ItemKind, active = true): string {
	if (!active) return 'color: var(--color-fg-muted);';
	const c = `var(--color-project-${KIND_COLOUR[k]})`;
	return `background-color: color-mix(in oklch, ${c} 18%, transparent); color: ${c}; border-color: ${c};`;
}

// Status chips use the accent colour (regardless of which status) — they're
// about "selected vs not," not a per-status palette. Same shape as kindChipStyle.
export function statusChipStyle(active = true): string {
	if (!active) return 'color: var(--color-fg-muted);';
	return 'background-color: color-mix(in oklch, var(--color-accent) 18%, transparent); color: var(--color-accent); border-color: var(--color-accent);';
}
