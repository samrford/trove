// Activity log — types mirroring the backend `data.Activity` JSON, plus the
// pure display logic that turns an event into a renderable descriptor. Kept
// out of the component so it's unit-testable with no DOM (see activity.test.ts).
//
// The locked rendering rules live here:
//  - single changed field → a specific verb ("renamed", "changed status")
//  - multiple changed fields → grouped "edited …" + per-field sub-lines
//  - status/kind changes expose the raw enum values so the component can draw
//    chips (StatusIcon + labels) rather than baking presentation in here
//  - `position` is recorded by the backend (MCP fidelity) but is display-noise:
//    a position-only update is classified as a reorder; surfaces hide those.

import type { ItemKind, ItemStatus } from '$lib/api/items';
import { KIND_LABEL, STATUS_LABEL } from '$lib/itemDisplay';

export type ActivityAction =
	| 'item.created'
	| 'item.updated'
	| 'item.deleted'
	| 'item.tag_added'
	| 'item.tag_removed'
	| 'attachment.added'
	| 'attachment.removed'
	| 'project.created'
	| 'project.updated'
	| 'project.deleted'
	| 'note';

export type ItemSnapshot = { seq: number; title: string; kind: ItemKind };
export type ProjectSnapshot = { slug: string; name: string };

// A diffed field is either an old/new pair, or — for long text (body,
// description) — a diff-match-patch patch, or a truncated marker when the
// input was too big to diff.
export type FieldDiff =
	| { old: unknown; new: unknown }
	| { diff: string }
	| { truncated: true; old_lines: number; new_lines: number };

export type ActivityPayload = {
	item?: ItemSnapshot;
	project?: ProjectSnapshot;
	diff?: Record<string, FieldDiff>;
	tag?: { slug: string; name: string };
	attachment?: { filename: string; size_bytes?: number; source?: string };
	text?: string;
};

export type Activity = {
	id: string;
	project_id: string;
	item_id: string | null;
	actor_id: string;
	action: ActivityAction;
	payload: ActivityPayload;
	created_at: string;
};

export type ActivityIcon =
	| 'created'
	| 'edited'
	| 'status'
	| 'kind'
	| 'tag'
	| 'untag'
	| 'attachment'
	| 'deleted'
	| 'project'
	| 'reorder'
	| 'note';

export type ActivityDescriptor = {
	icon: ActivityIcon;
	verb: string;
	/** Project-surface context: the item this happened to (`#seq`). */
	itemRef?: { seq: number; title: string };
	/** Single-field inline detail, e.g. `"Old" → "New"`. */
	detail?: string;
	/** Status change — component renders StatusIcon + labels. */
	statusChange?: { from: ItemStatus; to: ItemStatus };
	/** Kind change — component renders kind chips. */
	kindChange?: { from: ItemKind; to: ItemKind };
	/** Multi-field grouped detail, one human line per changed field. */
	subLines?: string[];
	/** Position-only update — display-noise; surfaces hide by default. */
	isReorder: boolean;
};

const TITLE_CLIP = 60;

function clip(s: string, n = TITLE_CLIP): string {
	return s.length > n ? s.slice(0, n - 1) + '…' : s;
}

function asStr(v: unknown): string {
	return typeof v === 'string' ? v : String(v ?? '');
}

function pair(d: FieldDiff): { old: unknown; new: unknown } | null {
	return d && 'old' in d ? { old: d.old, new: d.new } : null;
}

/** Diff field keys that are meaningful to a user (position is hidden). */
function visibleDiffKeys(diff: Record<string, FieldDiff> | undefined): string[] {
	if (!diff) return [];
	return Object.keys(diff).filter((k) => k !== 'position');
}

/** True when an item.updated changed only `position` (a bare reorder). */
export function isReorderActivity(a: Activity): boolean {
	if (a.action !== 'item.updated') return false;
	const keys = Object.keys(a.payload.diff ?? {});
	return keys.length > 0 && keys.every((k) => k === 'position');
}

function fieldSubLine(field: string, d: FieldDiff): string {
	if (field === 'body') return 'notes updated';
	if (field === 'description') return 'description updated';
	const p = pair(d);
	if (!p) return `${field} updated`;
	if (field === 'status') {
		return `status: ${STATUS_LABEL[p.old as ItemStatus] ?? asStr(p.old)} → ${STATUS_LABEL[p.new as ItemStatus] ?? asStr(p.new)}`;
	}
	if (field === 'kind') {
		return `kind: ${KIND_LABEL[p.old as ItemKind] ?? asStr(p.old)} → ${KIND_LABEL[p.new as ItemKind] ?? asStr(p.new)}`;
	}
	if (field === 'title') return `title: "${clip(asStr(p.old))}" → "${clip(asStr(p.new))}"`;
	return `${field}: ${clip(asStr(p.old))} → ${clip(asStr(p.new))}`;
}

function describeItemUpdated(a: Activity, item: ItemSnapshot): ActivityDescriptor {
	const diff = a.payload.diff ?? {};
	if (isReorderActivity(a)) {
		return { icon: 'reorder', verb: 'reordered', isReorder: true };
	}
	const keys = visibleDiffKeys(diff);
	if (keys.length === 1) {
		const f = keys[0];
		const p = pair(diff[f]);
		if (f === 'status' && p) {
			return {
				icon: 'status',
				verb: 'changed status',
				statusChange: { from: p.old as ItemStatus, to: p.new as ItemStatus },
				isReorder: false
			};
		}
		if (f === 'kind' && p) {
			return {
				icon: 'kind',
				verb: 'changed kind',
				kindChange: { from: p.old as ItemKind, to: p.new as ItemKind },
				isReorder: false
			};
		}
		if (f === 'title' && p) {
			return {
				icon: 'edited',
				verb: 'renamed',
				detail: `"${clip(asStr(p.old))}" → "${clip(asStr(p.new))}"`,
				isReorder: false
			};
		}
		if (f === 'body') {
			return { icon: 'edited', verb: 'edited the notes', isReorder: false };
		}
		return { icon: 'edited', verb: `edited ${f}`, isReorder: false };
	}
	// Multiple visible fields → grouped.
	return {
		icon: 'edited',
		verb: `edited this ${item.kind}`,
		subLines: keys.map((k) => fieldSubLine(k, diff[k])),
		isReorder: false
	};
}

function describeProjectUpdated(a: Activity): ActivityDescriptor {
	const diff = a.payload.diff ?? {};
	const keys = Object.keys(diff);
	if (keys.length === 1) {
		const f = keys[0];
		const p = pair(diff[f]);
		if (f === 'name' && p) {
			return {
				icon: 'project',
				verb: 'renamed the project',
				detail: `"${clip(asStr(p.old))}" → "${clip(asStr(p.new))}"`,
				isReorder: false
			};
		}
		if (f === 'description') {
			return { icon: 'project', verb: 'edited the project description', isReorder: false };
		}
		if (f === 'slug' && p) {
			return {
				icon: 'project',
				verb: 'changed the project URL',
				detail: `"${asStr(p.old)}" → "${asStr(p.new)}"`,
				isReorder: false
			};
		}
		return { icon: 'project', verb: 'restyled the project', isReorder: false };
	}
	return {
		icon: 'project',
		verb: 'edited the project',
		subLines: keys.map((k) => fieldSubLine(k, diff[k])),
		isReorder: false
	};
}

/**
 * describeActivity turns an event into a render-ready descriptor. The
 * component owns all markup; this owns all wording/classification.
 */
export function describeActivity(a: Activity): ActivityDescriptor {
	const item = a.payload.item;
	const ref = item ? { itemRef: { seq: item.seq, title: item.title } } : {};

	switch (a.action) {
		case 'item.created':
			return {
				icon: 'created',
				verb: `created this ${item?.kind ?? 'item'}`,
				...ref,
				isReorder: false
			};
		case 'item.deleted':
			return {
				icon: 'deleted',
				verb: `deleted this ${item?.kind ?? 'item'}`,
				...ref,
				isReorder: false
			};
		case 'item.updated':
			return { ...describeItemUpdated(a, item ?? { seq: 0, title: '', kind: 'task' }), ...ref };
		case 'item.tag_added':
			return {
				icon: 'tag',
				verb: `tagged ${a.payload.tag?.name ?? ''}`.trim(),
				...ref,
				isReorder: false
			};
		case 'item.tag_removed':
			return {
				icon: 'untag',
				verb: `untagged ${a.payload.tag?.name ?? ''}`.trim(),
				...ref,
				isReorder: false
			};
		case 'attachment.added':
			return {
				icon: 'attachment',
				verb: `attached ${a.payload.attachment?.filename ?? 'a file'}`,
				...ref,
				isReorder: false
			};
		case 'attachment.removed':
			return {
				icon: 'attachment',
				verb: `removed ${a.payload.attachment?.filename ?? 'an attachment'}`,
				...ref,
				isReorder: false
			};
		case 'project.created':
			return { icon: 'project', verb: 'created the project', isReorder: false };
		case 'project.updated':
			return describeProjectUpdated(a);
		case 'project.deleted':
			return { icon: 'project', verb: 'deleted the project', isReorder: false };
		case 'note':
			return { icon: 'note', verb: a.payload.text ?? 'added a note', ...ref, isReorder: false };
		default:
			return { icon: 'edited', verb: a.action, ...ref, isReorder: false };
	}
}

// --- Burst collapsing (panel/feed) ---

export type ActivityBurst = {
	kind: 'burst';
	actorId: string;
	entries: Activity[];
	/** Newest and oldest created_at in the burst (entries are newest-first). */
	latest: string;
	earliest: string;
};

export type ActivityRow = Activity | ActivityBurst;

export function isBurst(r: ActivityRow): r is ActivityBurst {
	return 'kind' in r && (r as ActivityBurst).kind === 'burst';
}

const BURST_WINDOW_MS = 5 * 60 * 1000;

/**
 * collapseBursts groups runs of consecutive same-actor events where each
 * adjacent pair is within `windowMs`, into one expandable burst. Input is
 * assumed newest-first (the API order). Pure: array → array.
 */
export function collapseBursts(entries: Activity[], windowMs = BURST_WINDOW_MS): ActivityRow[] {
	const out: ActivityRow[] = [];
	let run: Activity[] = [];

	const flush = () => {
		if (run.length === 0) return;
		if (run.length === 1) {
			out.push(run[0]);
		} else {
			out.push({
				kind: 'burst',
				actorId: run[0].actor_id,
				entries: run,
				latest: run[0].created_at,
				earliest: run[run.length - 1].created_at
			});
		}
		run = [];
	};

	for (const e of entries) {
		if (run.length === 0) {
			run = [e];
			continue;
		}
		const prev = run[run.length - 1];
		const sameActor = prev.actor_id === e.actor_id;
		const close =
			Math.abs(new Date(prev.created_at).getTime() - new Date(e.created_at).getTime()) <= windowMs;
		if (sameActor && close) {
			run.push(e);
		} else {
			flush();
			run = [e];
		}
	}
	flush();
	return out;
}

// --- Filtering ---

export type ActivityFilter = {
	actions?: ActivityAction[];
	itemId?: string;
	actorId?: string;
	/** When false (default), position-only reorders are hidden. */
	includeReorders?: boolean;
};

export function matchesFilter(a: Activity, f: ActivityFilter): boolean {
	if (f.actions && f.actions.length > 0 && !f.actions.includes(a.action)) return false;
	if (f.itemId && a.item_id !== f.itemId) return false;
	if (f.actorId && a.actor_id !== f.actorId) return false;
	if (!f.includeReorders && isReorderActivity(a)) return false;
	return true;
}
