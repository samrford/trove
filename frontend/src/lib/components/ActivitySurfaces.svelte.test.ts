// Surface wiring tests (vitest `client` project — chromium). listActivity is
// mocked so these assert rendering/filtering/CTA behaviour, not the network.
// The classification logic itself is covered by the pure unit tests.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { listActivity } from '$lib/api/activity';
import { makeActivity } from '$lib/activity.fixtures';
import ActivityRail from './ActivityRail.svelte';
import ActivityStrip from './ActivityStrip.svelte';
import ActivityPanel from './ActivityPanel.svelte';

vi.mock('$lib/api', () => ({
	errMsg: (e: unknown) => (e instanceof Error ? e.message : String(e))
}));
vi.mock('$lib/api/activity', () => ({ listActivity: vi.fn() }));

const mockList = vi.mocked(listActivity);
const ok = (activity: ReturnType<typeof makeActivity>[]) =>
	mockList.mockResolvedValue({ activity, next: null });

beforeEach(() => mockList.mockReset());

describe('ActivityRail', () => {
	it('renders events and hides bare reorders', async () => {
		ok([
			makeActivity({
				id: 'a',
				action: 'item.created',
				payload: { item: { seq: 1, title: 'X', kind: 'task' } }
			}),
			makeActivity({
				id: 'b',
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
});

describe('ActivityStrip', () => {
	it('caps at 3 and fires the CTA', async () => {
		ok(
			Array.from({ length: 6 }, (_, i) =>
				makeActivity({ id: `s${i}`, payload: { item: { seq: i, title: `T${i}`, kind: 'task' } } })
			)
		);
		const onOpenPanel = vi.fn();
		render(ActivityStrip, { slug: 'p1', onOpenPanel });
		// compact density renders `#seq` + verb (title is rich-only by design).
		await expect.element(page.getByText('#0', { exact: false })).toBeVisible();
		await expect.element(page.getByText('#5', { exact: false })).not.toBeInTheDocument();
		await page.getByRole('button', { name: /view all/i }).click();
		expect(onOpenPanel).toHaveBeenCalledOnce();
	});
});

describe('ActivityPanel', () => {
	it('renders nothing when closed', async () => {
		ok([makeActivity({})]);
		render(ActivityPanel, { open: false, slug: 'p1' });
		await expect
			.element(page.getByRole('link', { name: /view full history/i }))
			.not.toBeInTheDocument();
	});

	it('collapses a same-actor burst and links out to the full page', async () => {
		const burst = [0, 1, 2].map((i) =>
			makeActivity({
				id: `bz${i}`,
				actor_id: 'u1',
				created_at: `2026-05-17T12:0${i}:00.000Z`,
				action: 'item.updated',
				payload: {
					item: { seq: 1, title: 'X', kind: 'task' },
					diff: { status: { old: 'open', new: 'done' } }
				}
			})
		);
		ok(burst);
		render(ActivityPanel, { open: true, slug: 'garden' });

		// collapseBursts merges the 3 → one expandable summary. (Expand/collapse
		// behaviour itself is covered exhaustively by the collapseBursts unit
		// tests; here we just assert the panel wires it + links out.)
		await expect.element(page.getByText('You made 3 changes')).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: /view full history/i }))
			.toHaveAttribute('href', '/projects/garden/activity');
	});
});
