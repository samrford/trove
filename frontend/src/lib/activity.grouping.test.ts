// Pure tests for the dedicated-page helpers (server/node project). Kept
// separate from activity.test.ts to stay out of the hand-tuned cases there.
//
// Times are derived from a local `now` so the test is timezone- and
// locale-independent (we assert Today/Yesterday + structure, not a formatted
// date string).

import { describe, it, expect } from 'vitest';
import { groupByDay, actionsForGroups, ACTION_GROUPS } from './activity';
import { makeActivity } from './activity.fixtures';

function atLocal(base: Date, addDays: number, hour: number): string {
	const d = new Date(base);
	d.setDate(d.getDate() + addDays);
	d.setHours(hour, 0, 0, 0);
	return d.toISOString();
}

describe('groupByDay', () => {
	const now = new Date();
	now.setHours(15, 0, 0, 0);

	it('labels Today / Yesterday / other, grouped contiguously', () => {
		const groups = groupByDay(
			[
				makeActivity({ created_at: atLocal(now, 0, 14) }),
				makeActivity({ created_at: atLocal(now, 0, 9) }),
				makeActivity({ created_at: atLocal(now, -1, 12) }),
				makeActivity({ created_at: atLocal(now, -6, 12) })
			],
			now
		);
		expect(groups).toHaveLength(3);
		expect(groups[0].label).toBe('Today');
		expect(groups[0].entries).toHaveLength(2);
		expect(groups[1].label).toBe('Yesterday');
		expect(groups[1].entries).toHaveLength(1);
		expect(groups[2].label).not.toBe('Today');
		expect(groups[2].label).not.toBe('Yesterday');
		expect(groups[2].entries).toHaveLength(1);
	});

	it('empty in → empty out', () => {
		expect(groupByDay([], now)).toEqual([]);
	});
});

describe('actionsForGroups', () => {
	it('flattens + de-dupes selected buckets to API actions', () => {
		expect(actionsForGroups(['Tags'])).toEqual(['item.tag_added', 'item.tag_removed']);
		const both = actionsForGroups(['Created', 'Deleted']);
		expect(both).toEqual(
			expect.arrayContaining(['item.created', 'project.created', 'item.deleted', 'project.deleted'])
		);
		expect(both).toHaveLength(4);
	});

	it('unknown group name is ignored', () => {
		expect(actionsForGroups(['Nope'])).toEqual([]);
	});

	it('taxonomy buckets never overlap', () => {
		const all = Object.values(ACTION_GROUPS).flat();
		expect(new Set(all).size).toBe(all.length);
		expect(all).toContain('item.updated');
		expect(all).toContain('attachment.removed');
	});
});
