// Realtime — pure types + cursor utils + the reconciliation reducers. No
// DOM, no runes, no network — all the decision-making the live UI does lives
// here so it's exhaustively unit-testable in isolation.
//
// Two policies are encoded here::
//   1. Server-authoritative replace: an `item.changed` event carries the full
//      current Item; the local list is replaced.
//   2. Editor isolation: a remote change targeting an item whose editor is
//      open and *dirty* does NOT destroy the editor — it returns
//      `editorStale: true` so the surface can show a non-destructive
//      "updated elsewhere — reload" affordance instead.
//
// Out-of-order safety: every item carries a "lastSeen" cursor; an event with
// a cursor that isn't strictly newer is dropped (handles reconnect dups +
// any rare delivery reorder).

import type { Activity } from './activity';
import type { Item } from './api/items';
import type { Project } from './api/projects';

// --- Event shape (mirrors backend events.Message Event names) ---

export type ItemDeletedPayload = { id: string; seq: number; project_id: string };

export type RealtimeEvent =
	| { type: 'activity.added'; activity: Activity; cursor: string }
	| { type: 'item.changed'; item: Item; actorId?: string | null; cursor: string }
	| { type: 'item.deleted'; payload: ItemDeletedPayload; actorId?: string | null; cursor: string }
	| { type: 'project.changed'; project: Project; cursor: string }
	| { type: 'resync'; cursor: '' };

// --- Cursor (<RFC3339Nano>|<uuid>) ---

export type Cursor = { ts: number; id: string };

// parseCursor splits the wire string into (ts ms, id). Returns null on
// anything malformed — callers treat that as "no cursor seen yet".
export function parseCursor(s: string): Cursor | null {
	if (!s) return null;
	const i = s.lastIndexOf('|');
	if (i < 0) return null;
	const ts = Date.parse(s.slice(0, i));
	const id = s.slice(i + 1);
	if (Number.isNaN(ts) || !id) return null;
	return { ts, id };
}

// isNewer: `b` is strictly after `a`. Mirrors the backend's (created_at, id)
// keyset ordering. `a == null` (never seen) → anything's newer.
export function isNewer(a: Cursor | null, b: Cursor): boolean {
	if (!a) return true;
	if (b.ts !== a.ts) return b.ts > a.ts;
	return b.id > a.id;
}

// --- Editor isolation reference ---

export type EditorRef = { itemId: string; dirty: boolean } | null;

// --- Items reducer (per project) ---
//
// State carries the items list + a per-item lastSeen cursor map. The result
// adds `editorStale: true` when the event targeted the open dirty editor's
// item; the surface uses that to render the "updated elsewhere" affordance.
// `lastSeen` is intentionally NOT advanced in the editor-stale path — the
// user's reload re-applies the latest state cleanly.
//
// TODO (consumers, Stage 3/4): when handling the "reload" affordance, reset
// `lastSeen[item.id]` (or rebuild the map from the refetched items) so a
// reordered older event arriving post-reload can't clobber the fresh state.

export type ItemReducerState = {
	items: Item[];
	lastSeen: Record<string, string>; // itemID -> cursor string (opaque)
};

export type ItemReducerResult = ItemReducerState & {
	editorStale: boolean;
};

export function applyItemEvent(
	state: ItemReducerState,
	event: RealtimeEvent,
	editor: EditorRef
): ItemReducerResult {
	if (event.type === 'item.changed') {
		return reduceItemChanged(state, event.item, event.cursor, editor);
	}
	if (event.type === 'item.deleted') {
		return reduceItemDeleted(state, event.payload.id, event.cursor, editor);
	}
	// activity.added / project.changed / resync aren't this reducer's concern.
	return { ...state, editorStale: false };
}

function reduceItemChanged(
	state: ItemReducerState,
	item: Item,
	cursor: string,
	editor: EditorRef
): ItemReducerResult {
	if (isStale(state.lastSeen[item.id], cursor)) {
		return { ...state, editorStale: false };
	}
	if (isDirtyEditorTarget(editor, item.id)) {
		return { ...state, editorStale: true };
	}
	const idx = state.items.findIndex((it) => it.id === item.id);
	const items =
		idx >= 0 ? state.items.map((it, i) => (i === idx ? item : it)) : [...state.items, item];
	return {
		items,
		lastSeen: { ...state.lastSeen, [item.id]: cursor },
		editorStale: false
	};
}

function reduceItemDeleted(
	state: ItemReducerState,
	id: string,
	cursor: string,
	editor: EditorRef
): ItemReducerResult {
	if (isStale(state.lastSeen[id], cursor)) {
		return { ...state, editorStale: false };
	}
	if (isDirtyEditorTarget(editor, id)) {
		// Keep the item so the open editor stays valid; the affordance prompts
		// a reload that then applies the deletion cleanly.
		return { ...state, editorStale: true };
	}
	return {
		items: state.items.filter((it) => it.id !== id),
		lastSeen: { ...state.lastSeen, [id]: cursor },
		editorStale: false
	};
}

// Exported for surfaces that branch outside the reducers (e.g. the tags page's
// "lost the tag" drop path needs the same out-of-order safety the reducers
// apply internally).
export function isStale(prevCursor: string | undefined, incomingCursor: string): boolean {
	const incoming = parseCursor(incomingCursor);
	if (!incoming) return false; // can't tell — let it through
	const prev = parseCursor(prevCursor ?? '');
	return !isNewer(prev, incoming);
}

function isDirtyEditorTarget(editor: EditorRef, itemID: string): boolean {
	return !!editor && editor.itemId === itemID && editor.dirty;
}

// --- Project metadata reducer ---

export type ProjectReducerState = {
	project: Project;
	lastSeen: string;
};

export function applyProjectEvent(
	state: ProjectReducerState,
	event: RealtimeEvent
): ProjectReducerState {
	if (event.type !== 'project.changed') return state;
	if (event.project.id !== state.project.id) return state;
	if (isStale(state.lastSeen, event.cursor)) return state;
	return { project: event.project, lastSeen: event.cursor };
}

// --- Single-item editor reducer ---
//
// For QuickView / item detail / anywhere showing one live item: applies the
// same out-of-order + editor-isolation policy as applyItemEvent, but returns
// a discriminated `affordance` so the surface can render the right
// "updated/deleted elsewhere" banner. `item.deleted` always surfaces
// `deleted-elsewhere` for cross-actor deletes — we don't quietly yank the
// item out from under a reader (clean or dirty), per the locked policy.
//
// `isOwn` short-circuits both: when the event is the user's own echo (the
// caller computes this by comparing the event's actor_id to the current user
// id), the change applies silently and no banner is raised. This is the
// "actor's own echoed event is an idempotent no-op" rule from the spec.

export type EditorAffordance = 'none' | 'updated-elsewhere' | 'deleted-elsewhere';

export type EditorReducerResult = {
	item: Item;
	lastSeen: string;
	affordance: EditorAffordance;
};

export function applyEditorEvent(
	item: Item,
	lastSeen: string,
	event: RealtimeEvent,
	dirty: boolean,
	isOwn = false
): EditorReducerResult {
	if (event.type === 'item.changed') {
		if (event.item.id !== item.id) return { item, lastSeen, affordance: 'none' };
		if (isStale(lastSeen, event.cursor)) return { item, lastSeen, affordance: 'none' };
		if (dirty && !isOwn) {
			// Don't destroy the open editor; affordance prompts a reload.
			return { item, lastSeen, affordance: 'updated-elsewhere' };
		}
		// Clean editor, or our own echo — apply the server-authoritative state.
		return { item: event.item, lastSeen: event.cursor, affordance: 'none' };
	}
	if (event.type === 'item.deleted') {
		if (event.payload.id !== item.id) return { item, lastSeen, affordance: 'none' };
		if (isStale(lastSeen, event.cursor)) return { item, lastSeen, affordance: 'none' };
		if (isOwn) {
			// User deleted their own item; the local mutation handler has
			// already navigated / closed the panel. Just advance the cursor.
			return { item, lastSeen: event.cursor, affordance: 'none' };
		}
		// Keep the item in view (reader/editor stays valid); surface the notice.
		return { item, lastSeen: event.cursor, affordance: 'deleted-elsewhere' };
	}
	return { item, lastSeen, affordance: 'none' };
}

// --- Activity feed reducer ---
//
// Prepends `activity.added` rows; dedupes by activity id so a cross-boundary
// duplicate (catch-up + subscribe-before-catch-up) doesn't echo into the
// feed. Capped so a long session doesn't grow the array unbounded.

export function applyActivityFeed(feed: Activity[], event: RealtimeEvent, cap = 200): Activity[] {
	if (event.type !== 'activity.added') return feed;
	if (feed.some((a) => a.id === event.activity.id)) return feed;
	const next = [event.activity, ...feed];
	return next.length > cap ? next.slice(0, cap) : next;
}
