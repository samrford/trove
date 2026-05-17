import { describe, it, expect } from 'vitest';
import {
	describeActivity,
	isReorderActivity,
	collapseBursts,
	isBurst,
	matchesFilter
} from './activity';
import type { ActivityAction } from './activity';
import { makeActivity } from './activity.fixtures';

describe('describeActivity', () => {
	it('item.created → "created this {kind}" + itemRef', () => {
		const d = describeActivity(
			makeActivity({
				action: 'item.created',
				payload: { item: { seq: 42, title: 'Ship it', kind: 'task' } }
			})
		);
		expect(d.icon).toBe('created');
		expect(d.verb).toBe('created this task');
		expect(d.itemRef).toEqual({ seq: 42, title: 'Ship it' });
		expect(d.isReorder).toBe(false);
	});

	it('item.deleted → "deleted this {kind}"', () => {
		const d = describeActivity(
			makeActivity({
				action: 'item.deleted',
				payload: { item: { seq: 1, title: 'X', kind: 'brainstorm' } }
			})
		);
		expect(d.verb).toBe('deleted this brainstorm');
		expect(d.icon).toBe('deleted');
	});

	it('single status change → verb + statusChange (no inline detail)', () => {
		const d = describeActivity(
			makeActivity({
				action: 'item.updated',
				payload: {
					item: { seq: 1, title: 'X', kind: 'task' },
					diff: { status: { old: 'open', new: 'in_progress' } }
				}
			})
		);
		expect(d.verb).toBe('changed status');
		expect(d.icon).toBe('status');
		expect(d.statusChange).toEqual({ from: 'open', to: 'in_progress' });
		expect(d.isReorder).toBe(false);
	});

	it('single title change → "renamed" with clipped old → new detail', () => {
		const long = 'L'.repeat(100);
		const d = describeActivity(
			makeActivity({
				action: 'item.updated',
				payload: {
					item: { seq: 1, title: long, kind: 'task' },
					diff: { title: { old: 'old', new: long } }
				}
			})
		);
		expect(d.verb).toBe('renamed');
		expect(d.detail).toContain('old');
		expect(d.detail).toContain('…'); // long side clipped
	});

	it('single body change → "edited the notes" (diff stays in payload)', () => {
		const d = describeActivity(
			makeActivity({
				action: 'item.updated',
				payload: {
					item: { seq: 1, title: 'X', kind: 'task' },
					diff: { body: { diff: '@@ -1 +1 @@' } }
				}
			})
		);
		expect(d.verb).toBe('edited the notes');
		expect(d.subLines).toBeUndefined();
	});

	it('position-only update → reorder (hidden by surfaces)', () => {
		const d = describeActivity(
			makeActivity({
				action: 'item.updated',
				payload: {
					item: { seq: 1, title: 'X', kind: 'task' },
					diff: { position: { old: 1, new: 9 } }
				}
			})
		);
		expect(d.isReorder).toBe(true);
		expect(d.icon).toBe('reorder');
		expect(d.verb).toBe('reordered');
	});

	it('status + position → status change, NOT a reorder (position hidden)', () => {
		const d = describeActivity(
			makeActivity({
				action: 'item.updated',
				payload: {
					item: { seq: 1, title: 'X', kind: 'task' },
					diff: { status: { old: 'open', new: 'done' }, position: { old: 1, new: 9 } }
				}
			})
		);
		expect(d.verb).toBe('changed status');
		expect(d.isReorder).toBe(false);
	});

	it('multi-field → grouped "edited this {kind}" with a sub-line per field', () => {
		const d = describeActivity(
			makeActivity({
				action: 'item.updated',
				payload: {
					item: { seq: 1, title: 'New', kind: 'task' },
					diff: {
						title: { old: 'Old', new: 'New' },
						status: { old: 'open', new: 'done' },
						body: { diff: '@@' }
					}
				}
			})
		);
		expect(d.verb).toBe('edited this task');
		expect(d.subLines).toHaveLength(3);
		expect(d.subLines).toEqual(
			expect.arrayContaining(['title: "Old" → "New"', 'status: Open → Done', 'notes updated'])
		);
	});

	it('tag add/remove', () => {
		expect(
			describeActivity(
				makeActivity({
					action: 'item.tag_added',
					payload: { tag: { slug: 'urgent', name: 'urgent' } }
				})
			).verb
		).toBe('tagged urgent');
		expect(
			describeActivity(
				makeActivity({
					action: 'item.tag_removed',
					payload: { tag: { slug: 'urgent', name: 'urgent' } }
				})
			).verb
		).toBe('untagged urgent');
	});

	it('attachment add/remove', () => {
		expect(
			describeActivity(
				makeActivity({ action: 'attachment.added', payload: { attachment: { filename: 'a.png' } } })
			).verb
		).toBe('attached a.png');
		expect(
			describeActivity(
				makeActivity({
					action: 'attachment.removed',
					payload: { attachment: { filename: 'a.png' } }
				})
			).verb
		).toBe('removed a.png');
	});

	it('project created / renamed / multi-edit', () => {
		expect(describeActivity(makeActivity({ action: 'project.created', payload: {} })).verb).toBe(
			'created the project'
		);
		expect(
			describeActivity(
				makeActivity({
					action: 'project.updated',
					payload: { diff: { name: { old: 'A', new: 'B' } } }
				})
			).verb
		).toBe('renamed the project');
		const multi = describeActivity(
			makeActivity({
				action: 'project.updated',
				payload: { diff: { name: { old: 'A', new: 'B' }, description: { diff: '@@' } } }
			})
		);
		expect(multi.verb).toBe('edited the project');
		expect(multi.subLines).toEqual(expect.arrayContaining(['description updated']));
	});

	it('project slug change → "changed the project URL"', () => {
		const d = describeActivity(
			makeActivity({
				action: 'project.updated',
				payload: { diff: { slug: { old: 'old-slug', new: 'new-slug' } } }
			})
		);
		expect(d.verb).toBe('changed the project URL');
		expect(d.detail).toContain('old-slug');
		expect(d.detail).toContain('new-slug');
	});

	it('project colour/icon change → "restyled the project"', () => {
		expect(
			describeActivity(
				makeActivity({
					action: 'project.updated',
					payload: { diff: { colour: { old: 'plum', new: 'gold' } } }
				})
			).verb
		).toBe('restyled the project');
		expect(
			describeActivity(
				makeActivity({
					action: 'project.updated',
					payload: { diff: { icon: { old: 'a', new: 'b' } } }
				})
			).verb
		).toBe('restyled the project');
	});

	it('note → text as the verb, falling back to "added a note"', () => {
		expect(
			describeActivity(makeActivity({ action: 'note', payload: { text: 'Reviewed the design' } }))
				.verb
		).toBe('Reviewed the design');
		expect(describeActivity(makeActivity({ action: 'note', payload: {} })).verb).toBe(
			'added a note'
		);
	});

	it('attachment verbs fall back when filename is absent', () => {
		expect(describeActivity(makeActivity({ action: 'attachment.added', payload: {} })).verb).toBe(
			'attached a file'
		);
		expect(describeActivity(makeActivity({ action: 'attachment.removed', payload: {} })).verb).toBe(
			'removed an attachment'
		);
	});

	it('truncated body diff still reads as "edited the notes"', () => {
		const d = describeActivity(
			makeActivity({
				action: 'item.updated',
				payload: {
					item: { seq: 1, title: 'X', kind: 'task' },
					diff: { body: { truncated: true, old_lines: 200, new_lines: 240 } }
				}
			})
		);
		expect(d.verb).toBe('edited the notes');
		expect(d.icon).toBe('edited');
	});

	it('unknown action → raw action as the verb (default branch)', () => {
		const d = describeActivity(
			makeActivity({
				action: 'mystery.event' as ActivityAction,
				payload: { item: { seq: 3, title: 'Y', kind: 'task' } }
			})
		);
		expect(d.verb).toBe('mystery.event');
		expect(d.icon).toBe('edited');
		expect(d.itemRef).toEqual({ seq: 3, title: 'Y' });
	});
});

describe('isReorderActivity', () => {
	it('true only for position-only item.updated', () => {
		expect(
			isReorderActivity(
				makeActivity({
					action: 'item.updated',
					payload: { diff: { position: { old: 1, new: 2 } } }
				})
			)
		).toBe(true);
		expect(
			isReorderActivity(
				makeActivity({
					action: 'item.updated',
					payload: { diff: { position: { old: 1, new: 2 }, title: { old: 'a', new: 'b' } } }
				})
			)
		).toBe(false);
		expect(isReorderActivity(makeActivity({ action: 'item.created' }))).toBe(false);
	});
});

describe('collapseBursts', () => {
	const at = (iso: string, over: Partial<Parameters<typeof makeActivity>[0]> = {}) =>
		makeActivity({ created_at: iso, ...over });

	it('single entry passes through (not a burst)', () => {
		const rows = collapseBursts([at('2026-05-17T12:00:00.000Z')]);
		expect(rows).toHaveLength(1);
		expect(isBurst(rows[0])).toBe(false);
	});

	it('same actor within window collapses into one burst', () => {
		const rows = collapseBursts([
			at('2026-05-17T12:05:00.000Z'),
			at('2026-05-17T12:03:00.000Z'),
			at('2026-05-17T12:01:00.000Z')
		]);
		expect(rows).toHaveLength(1);
		expect(isBurst(rows[0])).toBe(true);
		if (isBurst(rows[0])) {
			expect(rows[0].entries).toHaveLength(3);
			expect(rows[0].latest).toBe('2026-05-17T12:05:00.000Z');
			expect(rows[0].earliest).toBe('2026-05-17T12:01:00.000Z');
		}
	});

	it('a different actor breaks the burst', () => {
		const rows = collapseBursts([
			at('2026-05-17T12:02:00.000Z', { actor_id: 'u1' }),
			at('2026-05-17T12:01:00.000Z', { actor_id: 'u2' })
		]);
		expect(rows).toHaveLength(2);
		expect(rows.every((r) => !isBurst(r))).toBe(true);
	});

	it('a gap larger than the window breaks the burst', () => {
		const rows = collapseBursts([at('2026-05-17T13:00:00.000Z'), at('2026-05-17T12:00:00.000Z')]);
		expect(rows).toHaveLength(2);
	});
});

describe('matchesFilter', () => {
	const reorder = makeActivity({
		action: 'item.updated',
		payload: { diff: { position: { old: 1, new: 2 } } }
	});

	it('filters by action set', () => {
		const a = makeActivity({ action: 'item.created' });
		expect(matchesFilter(a, { actions: ['item.created'] })).toBe(true);
		expect(matchesFilter(a, { actions: ['item.deleted'] })).toBe(false);
	});

	it('filters by item and actor', () => {
		const a = makeActivity({ item_id: 'i9', actor_id: 'u9' });
		expect(matchesFilter(a, { itemId: 'i9' })).toBe(true);
		expect(matchesFilter(a, { itemId: 'other' })).toBe(false);
		expect(matchesFilter(a, { actorId: 'u9' })).toBe(true);
		expect(matchesFilter(a, { actorId: 'nope' })).toBe(false);
	});

	it('hides reorders unless includeReorders', () => {
		expect(matchesFilter(reorder, {})).toBe(false);
		expect(matchesFilter(reorder, { includeReorders: true })).toBe(true);
	});
});
