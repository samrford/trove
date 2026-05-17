// Test fixture factory — shared by the pure unit tests and the ActivityEntry
// component tests so both speak the same shapes.

import type { Activity, ActivityAction, ActivityPayload } from './activity';

let seq = 0;

export function makeActivity(
	partial: Partial<Omit<Activity, 'action' | 'payload'>> & {
		action?: ActivityAction;
		payload?: ActivityPayload;
	} = {}
): Activity {
	seq += 1;
	return {
		id: partial.id ?? `act-${seq}`,
		project_id: partial.project_id ?? 'proj-1',
		item_id: partial.item_id ?? 'item-1',
		actor_id: partial.actor_id ?? 'user-1',
		action: partial.action ?? 'item.created',
		created_at: partial.created_at ?? '2026-05-17T12:00:00.000Z',
		payload: partial.payload ?? { item: { seq: 1, title: 'Test item', kind: 'task' } }
	};
}
