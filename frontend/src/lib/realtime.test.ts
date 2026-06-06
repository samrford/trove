// Realtime pure-logic tests. Exhaustive coverage of cursor parsing,
// out-of-order safety, server-authoritative replace, editor-isolation,
// project metadata, and activity feed dedup/cap.

import { describe, it, expect } from 'vitest';
import {
	parseCursor,
	isNewer,
	applyItemEvent,
	applyEditorEvent,
	applyProjectEvent,
	applyActivityFeed,
	type Cursor,
	type RealtimeEvent,
	type ItemReducerState,
	type EditorRef
} from './realtime';
import { makeActivity } from './activity.fixtures';
import type { Item } from './api/items';
import type { Project } from './api/projects';

// --- Fixtures ---

function makeItem(partial: Partial<Item> = {}): Item {
	return {
		id: 'i-1',
		project_id: 'p-1',
		sequence: 1,
		kind: 'task',
		status: 'open',
		title: 'Test item',
		body: null,
		position: 1000,
		creator_id: 'u-1',
		created_at: '2026-05-25T12:00:00.000Z',
		updated_at: '2026-05-25T12:00:00.000Z',
		tags: [],
		attachments: [],
		...partial
	};
}

function makeProject(partial: Partial<Project> = {}): Project {
	return {
		id: 'p-1',
		slug: 'test',
		name: 'Test project',
		description: null,
		colour: null,
		icon: null,
		owner_id: 'u-1',
		archived_at: null,
		created_at: '2026-05-25T12:00:00.000Z',
		updated_at: '2026-05-25T12:00:00.000Z',
		...partial
	};
}

// cur builds a cursor string at the given second offset + id.
function cur(secondOffset: number, id = 'a'): string {
	const base = Date.UTC(2026, 4, 25, 12, 0, secondOffset);
	return new Date(base).toISOString() + '|' + id;
}

function changed(item: Item, c: string): RealtimeEvent {
	return { type: 'item.changed', item, cursor: c };
}
function deleted(id: string, c: string, project_id = 'p-1', seq = 1): RealtimeEvent {
	return { type: 'item.deleted', payload: { id, seq, project_id }, cursor: c };
}

// --- parseCursor ---

describe('parseCursor', () => {
	it('parses a valid <ts>|<id>', () => {
		const c = parseCursor('2026-05-25T12:00:00.000Z|abc');
		expect(c).not.toBeNull();
		expect(c!.id).toBe('abc');
		expect(c!.ts).toBe(Date.UTC(2026, 4, 25, 12, 0, 0));
	});

	it('returns null for an empty string', () => {
		expect(parseCursor('')).toBeNull();
	});

	it('returns null when the pipe is missing', () => {
		expect(parseCursor('2026-05-25T12:00:00Z')).toBeNull();
	});

	it('returns null on an unparseable timestamp', () => {
		expect(parseCursor('not-a-date|abc')).toBeNull();
	});

	it('returns null when the id is empty', () => {
		expect(parseCursor('2026-05-25T12:00:00.000Z|')).toBeNull();
	});
});

// --- isNewer ---

describe('isNewer', () => {
	const a: Cursor = { ts: 1000, id: 'a' };

	it('null prev → anything is newer', () => {
		expect(isNewer(null, a)).toBe(true);
	});

	it('strictly later ts → newer', () => {
		expect(isNewer(a, { ts: 2000, id: 'a' })).toBe(true);
	});

	it('strictly earlier ts → not newer', () => {
		expect(isNewer(a, { ts: 500, id: 'a' })).toBe(false);
	});

	it('same ts, lex-greater id → newer', () => {
		expect(isNewer(a, { ts: 1000, id: 'b' })).toBe(true);
	});

	it('same ts, lex-lesser id → not newer', () => {
		expect(isNewer({ ts: 1000, id: 'b' }, { ts: 1000, id: 'a' })).toBe(false);
	});

	it('identical cursor → not newer (not strictly after itself)', () => {
		expect(isNewer(a, { ...a })).toBe(false);
	});
});

// --- applyItemEvent — item.changed ---

describe('applyItemEvent / item.changed', () => {
	const empty: ItemReducerState = { items: [], lastSeen: {} };

	it('adds an item when unseen', () => {
		const it = makeItem();
		const r = applyItemEvent(empty, changed(it, cur(1)), null);
		expect(r.items).toEqual([it]);
		expect(r.lastSeen[it.id]).toBe(cur(1));
		expect(r.editorStale).toBe(false);
	});

	it('replaces an existing item by id (preserves order)', () => {
		const a = makeItem({ id: 'a' });
		const b = makeItem({ id: 'b' });
		const state: ItemReducerState = { items: [a, b], lastSeen: {} };
		const bUpdated = makeItem({ id: 'b', title: 'changed' });
		const r = applyItemEvent(state, changed(bUpdated, cur(1)), null);
		expect(r.items.map((i) => i.id)).toEqual(['a', 'b']);
		expect(r.items[1]).toEqual(bUpdated);
	});

	it('drops an out-of-order event (older cursor than lastSeen)', () => {
		const it = makeItem({ id: 'x' });
		const state: ItemReducerState = {
			items: [it],
			lastSeen: { x: cur(10) }
		};
		const stale = makeItem({ id: 'x', title: 'stale' });
		const r = applyItemEvent(state, changed(stale, cur(5)), null);
		expect(r.items[0].title).toBe(it.title);
		expect(r.lastSeen.x).toBe(cur(10)); // unchanged
		expect(r.editorStale).toBe(false);
	});

	it('drops a duplicate event (identical cursor)', () => {
		const it = makeItem({ id: 'x' });
		const state: ItemReducerState = { items: [it], lastSeen: { x: cur(5) } };
		const dupe = makeItem({ id: 'x', title: 'dupe' });
		const r = applyItemEvent(state, changed(dupe, cur(5)), null);
		expect(r.items[0].title).toBe(it.title);
	});

	it('flags editorStale + leaves items/lastSeen alone when the dirty editor targets this item', () => {
		const it = makeItem({ id: 'x', title: 'original' });
		const state: ItemReducerState = { items: [it], lastSeen: {} };
		const remote = makeItem({ id: 'x', title: 'remote' });
		const editor: EditorRef = { itemId: 'x', dirty: true };
		const r = applyItemEvent(state, changed(remote, cur(1)), editor);
		expect(r.items[0].title).toBe('original');
		expect(r.lastSeen.x).toBeUndefined(); // not advanced — reload will resync
		expect(r.editorStale).toBe(true);
	});

	it('applies normally when the editor is clean (not dirty)', () => {
		const it = makeItem({ id: 'x', title: 'old' });
		const state: ItemReducerState = { items: [it], lastSeen: {} };
		const remote = makeItem({ id: 'x', title: 'new' });
		const editor: EditorRef = { itemId: 'x', dirty: false };
		const r = applyItemEvent(state, changed(remote, cur(1)), editor);
		expect(r.items[0].title).toBe('new');
		expect(r.editorStale).toBe(false);
	});

	it('applies normally when the editor is on a different item', () => {
		const x = makeItem({ id: 'x', title: 'old' });
		const state: ItemReducerState = { items: [x], lastSeen: {} };
		const editor: EditorRef = { itemId: 'OTHER', dirty: true };
		const r = applyItemEvent(state, changed(makeItem({ id: 'x', title: 'new' }), cur(1)), editor);
		expect(r.items[0].title).toBe('new');
		expect(r.editorStale).toBe(false);
	});
});

// --- applyItemEvent — item.deleted ---

describe('applyItemEvent / item.deleted', () => {
	it('removes an existing item by id', () => {
		const a = makeItem({ id: 'a' });
		const b = makeItem({ id: 'b' });
		const state: ItemReducerState = { items: [a, b], lastSeen: {} };
		const r = applyItemEvent(state, deleted('b', cur(1)), null);
		expect(r.items.map((i) => i.id)).toEqual(['a']);
		expect(r.lastSeen.b).toBe(cur(1));
	});

	it('idempotent for an item we never had', () => {
		const a = makeItem({ id: 'a' });
		const state: ItemReducerState = { items: [a], lastSeen: {} };
		const r = applyItemEvent(state, deleted('ghost', cur(1)), null);
		expect(r.items).toEqual([a]);
		expect(r.lastSeen.ghost).toBe(cur(1));
	});

	it('drops an out-of-order delete', () => {
		const a = makeItem({ id: 'a' });
		const state: ItemReducerState = { items: [a], lastSeen: { a: cur(10) } };
		const r = applyItemEvent(state, deleted('a', cur(5)), null);
		expect(r.items).toEqual([a]);
		expect(r.lastSeen.a).toBe(cur(10));
	});

	it('keeps the item + flags editorStale when the dirty editor targets it', () => {
		const a = makeItem({ id: 'a' });
		const state: ItemReducerState = { items: [a], lastSeen: {} };
		const editor: EditorRef = { itemId: 'a', dirty: true };
		const r = applyItemEvent(state, deleted('a', cur(1)), editor);
		expect(r.items).toEqual([a]); // not removed
		expect(r.editorStale).toBe(true);
		expect(r.lastSeen.a).toBeUndefined();
	});

	it('removes when the editor is clean', () => {
		const a = makeItem({ id: 'a' });
		const state: ItemReducerState = { items: [a], lastSeen: {} };
		const editor: EditorRef = { itemId: 'a', dirty: false };
		const r = applyItemEvent(state, deleted('a', cur(1)), editor);
		expect(r.items).toEqual([]);
		expect(r.editorStale).toBe(false);
	});
});

// --- applyItemEvent — other event types ---

describe('applyItemEvent / unrelated event types', () => {
	it('is a no-op for activity.added / project.changed / resync', () => {
		const a = makeItem({ id: 'a' });
		const state: ItemReducerState = { items: [a], lastSeen: { a: cur(1) } };
		const events: RealtimeEvent[] = [
			{ type: 'activity.added', activity: makeActivity({}), cursor: cur(2) },
			{ type: 'project.changed', project: makeProject(), cursor: cur(2) },
			{ type: 'resync', cursor: '' }
		];
		for (const e of events) {
			const r = applyItemEvent(state, e, null);
			expect(r.items).toEqual([a]);
			expect(r.lastSeen).toEqual({ a: cur(1) });
			expect(r.editorStale).toBe(false);
		}
	});
});

// --- applyEditorEvent (single-item editor reducer) ---

describe('applyEditorEvent', () => {
	const it1 = makeItem({ id: 'x', title: 'baseline' });

	it('clean editor + item.changed → replaces item, advances cursor, no affordance', () => {
		const next = makeItem({ id: 'x', title: 'fresh' });
		const r = applyEditorEvent(
			it1,
			'',
			{ type: 'item.changed', item: next, cursor: cur(1) },
			false
		);
		expect(r.item.title).toBe('fresh');
		expect(r.lastSeen).toBe(cur(1));
		expect(r.affordance).toBe('none');
	});

	it('dirty editor + item.changed → keeps item + lastSeen, surfaces updated-elsewhere', () => {
		const next = makeItem({ id: 'x', title: 'remote' });
		const r = applyEditorEvent(it1, '', { type: 'item.changed', item: next, cursor: cur(1) }, true);
		expect(r.item.title).toBe('baseline');
		expect(r.lastSeen).toBe(''); // not advanced — reload will resync
		expect(r.affordance).toBe('updated-elsewhere');
	});

	it('item.changed for a different id → no-op', () => {
		const other = makeItem({ id: 'OTHER', title: 'someone else' });
		const r = applyEditorEvent(
			it1,
			'',
			{ type: 'item.changed', item: other, cursor: cur(1) },
			false
		);
		expect(r.item).toBe(it1);
		expect(r.affordance).toBe('none');
	});

	it('stale item.changed → no-op (no affordance, cursor stays)', () => {
		const next = makeItem({ id: 'x', title: 'stale' });
		const r = applyEditorEvent(
			it1,
			cur(10),
			{ type: 'item.changed', item: next, cursor: cur(5) },
			false
		);
		expect(r.item.title).toBe('baseline');
		expect(r.lastSeen).toBe(cur(10));
		expect(r.affordance).toBe('none');
	});

	it('item.deleted (clean) → keeps item in view, surfaces deleted-elsewhere', () => {
		const r = applyEditorEvent(
			it1,
			'',
			{
				type: 'item.deleted',
				payload: { id: 'x', seq: 1, project_id: 'p-1' },
				cursor: cur(1)
			},
			false
		);
		expect(r.item).toBe(it1); // not removed
		expect(r.lastSeen).toBe(cur(1));
		expect(r.affordance).toBe('deleted-elsewhere');
	});

	it('item.deleted (dirty) → keeps item, surfaces deleted-elsewhere', () => {
		const r = applyEditorEvent(
			it1,
			'',
			{
				type: 'item.deleted',
				payload: { id: 'x', seq: 1, project_id: 'p-1' },
				cursor: cur(1)
			},
			true
		);
		expect(r.item).toBe(it1);
		expect(r.affordance).toBe('deleted-elsewhere');
	});

	it('item.deleted for a different id → no-op', () => {
		const r = applyEditorEvent(
			it1,
			'',
			{
				type: 'item.deleted',
				payload: { id: 'OTHER', seq: 2, project_id: 'p-1' },
				cursor: cur(1)
			},
			false
		);
		expect(r.item).toBe(it1);
		expect(r.affordance).toBe('none');
	});

	it('stale item.deleted → no-op', () => {
		const r = applyEditorEvent(
			it1,
			cur(10),
			{
				type: 'item.deleted',
				payload: { id: 'x', seq: 1, project_id: 'p-1' },
				cursor: cur(5)
			},
			false
		);
		expect(r.affordance).toBe('none');
		expect(r.lastSeen).toBe(cur(10));
	});

	it('own item.changed while dirty → silent sync, no affordance ("actor\'s own echo is a no-op")', () => {
		const next = makeItem({ id: 'x', title: 'my-own-tag-add' });
		const r = applyEditorEvent(
			it1,
			'',
			{ type: 'item.changed', item: next, cursor: cur(1) },
			true,
			true
		);
		expect(r.item.title).toBe('my-own-tag-add'); // applied
		expect(r.lastSeen).toBe(cur(1)); // advanced
		expect(r.affordance).toBe('none'); // no banner on your own action
	});

	it('own item.deleted → no affordance (the local handler closed the panel already)', () => {
		const r = applyEditorEvent(
			it1,
			'',
			{
				type: 'item.deleted',
				payload: { id: 'x', seq: 1, project_id: 'p-1' },
				cursor: cur(1)
			},
			false,
			true
		);
		expect(r.affordance).toBe('none');
		expect(r.lastSeen).toBe(cur(1));
	});

	it('unrelated event types (activity.added / project.changed / resync) → no-op', () => {
		const events: RealtimeEvent[] = [
			{ type: 'activity.added', activity: makeActivity({}), cursor: cur(1) },
			{ type: 'project.changed', project: makeProject(), cursor: cur(1) },
			{ type: 'resync', cursor: '' }
		];
		for (const e of events) {
			const r = applyEditorEvent(it1, cur(0), e, false);
			expect(r.item).toBe(it1);
			expect(r.lastSeen).toBe(cur(0));
			expect(r.affordance).toBe('none');
		}
	});
});

// --- applyProjectEvent ---

describe('applyProjectEvent', () => {
	const p = makeProject({ name: 'before' });

	it('replaces the project on project.changed for matching id', () => {
		const next = makeProject({ name: 'after' });
		const r = applyProjectEvent(
			{ project: p, lastSeen: '' },
			{ type: 'project.changed', project: next, cursor: cur(1) }
		);
		expect(r.project.name).toBe('after');
		expect(r.lastSeen).toBe(cur(1));
	});

	it('ignores project.changed for a different project id', () => {
		const other = makeProject({ id: 'other', name: 'other' });
		const r = applyProjectEvent(
			{ project: p, lastSeen: '' },
			{ type: 'project.changed', project: other, cursor: cur(1) }
		);
		expect(r.project).toBe(p);
	});

	it('drops an out-of-order project event', () => {
		const next = makeProject({ name: 'stale' });
		const r = applyProjectEvent(
			{ project: p, lastSeen: cur(10) },
			{ type: 'project.changed', project: next, cursor: cur(5) }
		);
		expect(r.project.name).toBe('before');
		expect(r.lastSeen).toBe(cur(10));
	});

	it('is a no-op for non-project events', () => {
		const state = { project: p, lastSeen: cur(1) };
		const r = applyProjectEvent(state, {
			type: 'item.deleted',
			payload: { id: 'x', seq: 1, project_id: 'p-1' },
			cursor: cur(2)
		});
		expect(r).toBe(state);
	});
});

// --- applyActivityFeed ---

describe('applyActivityFeed', () => {
	it('prepends activity.added in newest-first order', () => {
		const a = makeActivity({ id: 'a' });
		const b = makeActivity({ id: 'b' });
		const out = applyActivityFeed([a], {
			type: 'activity.added',
			activity: b,
			cursor: cur(2)
		});
		expect(out.map((x) => x.id)).toEqual(['b', 'a']);
	});

	it('dedupes by activity id (catch-up + live boundary)', () => {
		const a = makeActivity({ id: 'a' });
		const dupe = makeActivity({ id: 'a', payload: { text: 'looks different but same id' } });
		const out = applyActivityFeed([a], {
			type: 'activity.added',
			activity: dupe,
			cursor: cur(2)
		});
		expect(out).toEqual([a]);
	});

	it('caps the feed length, dropping the oldest', () => {
		const seed = Array.from({ length: 3 }, (_, i) => makeActivity({ id: `s-${i}` }));
		const fresh = makeActivity({ id: 'fresh' });
		const out = applyActivityFeed(
			seed,
			{
				type: 'activity.added',
				activity: fresh,
				cursor: cur(2)
			},
			3
		);
		expect(out.map((x) => x.id)).toEqual(['fresh', 's-0', 's-1']);
	});

	it('is a no-op for non-activity-added events', () => {
		const a = makeActivity({ id: 'a' });
		const out = applyActivityFeed([a], { type: 'resync', cursor: '' });
		expect(out).toEqual([a]);
	});

	it('preserves the feed when it would still fit under the cap', () => {
		const a = makeActivity({ id: 'a' });
		const b = makeActivity({ id: 'b' });
		const out = applyActivityFeed(
			[a],
			{ type: 'activity.added', activity: b, cursor: cur(2) },
			200
		);
		expect(out.length).toBe(2);
	});
});
