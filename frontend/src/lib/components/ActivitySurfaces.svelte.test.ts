// Surface wiring tests (vitest `client` project — chromium). listActivity is
// mocked so these assert rendering/filtering/CTA behaviour, not the network.
// The realtime store is mocked too so tests can synthesise live events and
// drive the surfaces' subscriptions. Classification logic itself is covered by
// the pure unit tests in realtime.test.ts + activity.test.ts.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { listActivity } from '$lib/api/activity';
import { makeActivity } from '$lib/activity.fixtures';
import type { RealtimeEvent } from '$lib/realtime';
import ActivityRail from './ActivityRail.svelte';
import ActivityStrip from './ActivityStrip.svelte';
import ActivityPanel from './ActivityPanel.svelte';

vi.mock('$lib/api', () => ({
	errMsg: (e: unknown) => (e instanceof Error ? e.message : String(e))
}));
vi.mock('$lib/api/activity', () => ({ listActivity: vi.fn() }));

// Controllable realtime mock. Tests register listeners through the component
// (which calls `realtime.on(...)`), then `emit(type, event)` to drive them.
const listeners = new Map<string, Set<(e: RealtimeEvent) => void>>();
function addListener(type: string, fn: (e: RealtimeEvent) => void) {
	let set = listeners.get(type);
	if (!set) {
		set = new Set();
		listeners.set(type, set);
	}
	set.add(fn);
	return () => set.delete(fn);
}
function emit(type: RealtimeEvent['type'], event: RealtimeEvent) {
	listeners.get(type)?.forEach((f) => f(event));
}
vi.mock('$lib/realtime.svelte', () => ({
	realtime: { on: addListener, status: 'connected' }
}));

const mockList = vi.mocked(listActivity);
const ok = (activity: ReturnType<typeof makeActivity>[]) =>
	mockList.mockResolvedValue({ activity, next: null });

// Hold the catch-up promise pending so a test can synthesize a live event
// before the fetch resolves — the race the surfaces' pending-buffer closes.
function deferred(): { resolve: (activity: ReturnType<typeof makeActivity>[]) => void } {
	let resolveFn!: (a: ReturnType<typeof makeActivity>[]) => void;
	mockList.mockImplementation(
		() =>
			new Promise((res) => {
				resolveFn = (activity) => res({ activity, next: null });
			})
	);
	return { resolve: (activity) => resolveFn(activity) };
}

const PROJECT = { id: 'p-uuid', slug: 'p1' };

beforeEach(() => {
	mockList.mockReset();
	listeners.clear();
});

// --- ActivityRail (item-scoped) ---

describe('ActivityRail', () => {
	it('renders catch-up events and hides bare reorders', async () => {
		ok([
			makeActivity({
				id: 'a',
				item_id: 'i1',
				action: 'item.created',
				payload: { item: { seq: 1, title: 'X', kind: 'task' } }
			}),
			makeActivity({
				id: 'b',
				item_id: 'i1',
				action: 'item.updated',
				payload: {
					item: { seq: 1, title: 'X', kind: 'task' },
					diff: { position: { old: 1, new: 9 } }
				}
			})
		]);
		render(ActivityRail, { slug: 'p1', itemId: 'i1' });
		await expect.element(page.getByText('created this task')).toBeVisible();
		await expect.element(page.getByText('reordered')).not.toBeInTheDocument();
	});

	it('shows the empty state', async () => {
		ok([]);
		render(ActivityRail, { slug: 'p1', itemId: 'i1' });
		await expect.element(page.getByText(/nothing's happened/i)).toBeVisible();
	});

	it('prepends a live activity.added for the matching item', async () => {
		ok([]);
		render(ActivityRail, { slug: 'p1', itemId: 'i1' });
		await expect.element(page.getByText(/nothing's happened/i)).toBeVisible();

		emit('activity.added', {
			type: 'activity.added',
			activity: makeActivity({
				id: 'live',
				item_id: 'i1',
				action: 'item.updated',
				payload: {
					item: { seq: 4, title: 'live', kind: 'task' },
					diff: { title: { old: 'old', new: 'live' } }
				}
			}),
			cursor: ''
		});
		await expect.element(page.getByText(/renamed/i)).toBeVisible();
	});

	it('ignores activity.added for a different item', async () => {
		ok([]);
		render(ActivityRail, { slug: 'p1', itemId: 'i1' });
		await expect.element(page.getByText(/nothing's happened/i)).toBeVisible();

		emit('activity.added', {
			type: 'activity.added',
			activity: makeActivity({
				id: 'other',
				item_id: 'i2',
				action: 'item.created',
				payload: { item: { seq: 7, title: 'other', kind: 'task' } }
			}),
			cursor: ''
		});
		// Empty state remains — the other item's event was filtered out.
		await expect.element(page.getByText(/nothing's happened/i)).toBeVisible();
	});

	it('merges a live event that arrives during the catch-up fetch', async () => {
		const d = deferred();
		render(ActivityRail, { slug: 'p1', itemId: 'i1' });

		// Emit while catch-up is still pending — should land in pending buffer.
		emit('activity.added', {
			type: 'activity.added',
			activity: makeActivity({
				id: 'mid',
				item_id: 'i1',
				action: 'item.updated',
				payload: {
					item: { seq: 9, title: 'mid-flight', kind: 'task' },
					diff: { title: { old: 'was', new: 'mid-flight' } }
				}
			}),
			cursor: ''
		});

		// Resolve catch-up with a *different* event; the in-flight event must
		// survive the merge rather than be overwritten.
		d.resolve([
			makeActivity({
				id: 'caught',
				item_id: 'i1',
				action: 'item.created',
				payload: { item: { seq: 1, title: 'caught', kind: 'task' } }
			})
		]);

		await expect.element(page.getByText('created this task')).toBeVisible();
		await expect.element(page.getByText(/renamed/i)).toBeVisible();
	});

	it('drops live reorder events at ingress', async () => {
		ok([]);
		render(ActivityRail, { slug: 'p1', itemId: 'i1' });
		await expect.element(page.getByText(/nothing's happened/i)).toBeVisible();

		emit('activity.added', {
			type: 'activity.added',
			activity: makeActivity({
				id: 'live-reorder',
				item_id: 'i1',
				action: 'item.updated',
				payload: {
					item: { seq: 3, title: 'X', kind: 'task' },
					diff: { position: { old: 1, new: 9 } }
				}
			}),
			cursor: ''
		});
		// Buffer stayed empty — reorders never enter the surface.
		await expect.element(page.getByText(/nothing's happened/i)).toBeVisible();
	});
});

// --- ActivityStrip (project-scoped) ---

describe('ActivityStrip', () => {
	it('caps at 3 and fires the CTA', async () => {
		ok(
			Array.from({ length: 6 }, (_, i) =>
				makeActivity({
					id: `s${i}`,
					project_id: PROJECT.id,
					payload: { item: { seq: i, title: `T${i}`, kind: 'task' } }
				})
			)
		);
		const onOpenPanel = vi.fn();
		render(ActivityStrip, { project: PROJECT, onOpenPanel });
		await expect.element(page.getByText('#0', { exact: false })).toBeVisible();
		await expect.element(page.getByText('#5', { exact: false })).not.toBeInTheDocument();
		await page.getByRole('button', { name: /view all/i }).click();
		expect(onOpenPanel).toHaveBeenCalledOnce();
	});

	it('prepends a live activity.added for the matching project', async () => {
		ok([]);
		render(ActivityStrip, { project: PROJECT, onOpenPanel: vi.fn() });
		await expect.element(page.getByText(/nothing's happened/i)).toBeVisible();

		emit('activity.added', {
			type: 'activity.added',
			activity: makeActivity({
				id: 'live',
				project_id: PROJECT.id,
				action: 'item.created',
				payload: { item: { seq: 9, title: 'live!', kind: 'task' } }
			}),
			cursor: ''
		});
		await expect.element(page.getByText('#9', { exact: false })).toBeVisible();
	});

	it('ignores activity.added for a different project', async () => {
		ok([]);
		render(ActivityStrip, { project: PROJECT, onOpenPanel: vi.fn() });

		emit('activity.added', {
			type: 'activity.added',
			activity: makeActivity({
				id: 'other-proj',
				project_id: 'OTHER',
				action: 'item.created',
				payload: { item: { seq: 99, title: 'somewhere else', kind: 'task' } }
			}),
			cursor: ''
		});
		await expect.element(page.getByText(/nothing's happened/i)).toBeVisible();
	});

	it('merges a live event that arrives during the catch-up fetch', async () => {
		const d = deferred();
		render(ActivityStrip, { project: PROJECT, onOpenPanel: vi.fn() });

		emit('activity.added', {
			type: 'activity.added',
			activity: makeActivity({
				id: 'mid',
				project_id: PROJECT.id,
				action: 'item.created',
				payload: { item: { seq: 7, title: 'mid-flight', kind: 'task' } }
			}),
			cursor: ''
		});

		d.resolve([
			makeActivity({
				id: 'caught',
				project_id: PROJECT.id,
				action: 'item.created',
				payload: { item: { seq: 1, title: 'caught', kind: 'task' } }
			})
		]);

		await expect.element(page.getByText('#7', { exact: false })).toBeVisible();
		await expect.element(page.getByText('#1', { exact: false })).toBeVisible();
	});

	it('drops live reorder events at ingress', async () => {
		ok([]);
		render(ActivityStrip, { project: PROJECT, onOpenPanel: vi.fn() });
		await expect.element(page.getByText(/nothing's happened/i)).toBeVisible();

		emit('activity.added', {
			type: 'activity.added',
			activity: makeActivity({
				id: 'live-reorder',
				project_id: PROJECT.id,
				action: 'item.updated',
				payload: {
					item: { seq: 3, title: 'X', kind: 'task' },
					diff: { position: { old: 1, new: 9 } }
				}
			}),
			cursor: ''
		});
		await expect.element(page.getByText(/nothing's happened/i)).toBeVisible();
	});
});

// --- ActivityPanel ---

describe('ActivityPanel', () => {
	it('renders nothing when closed', async () => {
		ok([makeActivity({})]);
		render(ActivityPanel, { open: false, project: PROJECT });
		await expect
			.element(page.getByRole('link', { name: /view full history/i }))
			.not.toBeInTheDocument();
	});

	it('collapses a same-actor burst and links out to the full page', async () => {
		const burst = [0, 1, 2].map((i) =>
			makeActivity({
				id: `bz${i}`,
				actor_id: 'u1',
				project_id: PROJECT.id,
				created_at: `2026-05-17T12:0${i}:00.000Z`,
				action: 'item.updated',
				payload: {
					item: { seq: 1, title: 'X', kind: 'task' },
					diff: { status: { old: 'open', new: 'done' } }
				}
			})
		);
		ok(burst);
		render(ActivityPanel, { open: true, project: { id: PROJECT.id, slug: 'garden' } });
		await expect.element(page.getByText('You made 3 changes')).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: /view full history/i }))
			.toHaveAttribute('href', '/projects/garden/activity');
	});

	it('prepends a live activity.added when open', async () => {
		ok([]);
		render(ActivityPanel, { open: true, project: PROJECT });
		await expect.element(page.getByText(/nothing's happened/i)).toBeVisible();

		emit('activity.added', {
			type: 'activity.added',
			activity: makeActivity({
				id: 'live',
				project_id: PROJECT.id,
				action: 'item.created',
				payload: { item: { seq: 42, title: 'fresh', kind: 'task' } }
			}),
			cursor: ''
		});
		await expect.element(page.getByText('#42', { exact: false })).toBeVisible();
	});

	it('merges a live event that arrives during the catch-up fetch', async () => {
		const d = deferred();
		render(ActivityPanel, { open: true, project: PROJECT });

		// Same actor + same created_at → collapseBursts merges these into one
		// summary. Asserting "You made 2 changes" both confirms the merge ran
		// (it wouldn't say 2 if the live event had been overwritten) and
		// exercises the burst pipeline on a mixed catch-up+live source.
		emit('activity.added', {
			type: 'activity.added',
			activity: makeActivity({
				id: 'mid',
				project_id: PROJECT.id,
				action: 'item.created',
				payload: { item: { seq: 7, title: 'mid-flight', kind: 'task' } }
			}),
			cursor: ''
		});

		d.resolve([
			makeActivity({
				id: 'caught',
				project_id: PROJECT.id,
				action: 'item.created',
				payload: { item: { seq: 1, title: 'caught', kind: 'task' } }
			})
		]);

		await expect.element(page.getByText('You made 2 changes')).toBeVisible();
	});

	it('drops live reorder events at ingress (no badge bump, no entry growth)', async () => {
		ok([]);
		render(ActivityPanel, { open: true, project: PROJECT });
		await expect.element(page.getByText(/nothing's happened/i)).toBeVisible();

		emit('activity.added', {
			type: 'activity.added',
			activity: makeActivity({
				id: 'live-reorder',
				project_id: PROJECT.id,
				action: 'item.updated',
				payload: {
					item: { seq: 3, title: 'X', kind: 'task' },
					diff: { position: { old: 1, new: 9 } }
				}
			}),
			cursor: ''
		});
		await expect.element(page.getByText(/nothing's happened/i)).toBeVisible();
	});
});
