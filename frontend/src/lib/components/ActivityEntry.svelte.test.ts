// Component tests (vitest `client` project — real chromium via Playwright).
// Asserts the prop-driven behaviour through rendered DOM, not internals.

import { describe, it, expect } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import ActivityEntry from './ActivityEntry.svelte';
import { makeActivity } from '$lib/activity.fixtures';

describe('ActivityEntry', () => {
	it('renders "You" + the verb for item.created', async () => {
		render(ActivityEntry, {
			entry: makeActivity({
				action: 'item.created',
				payload: { item: { seq: 42, title: 'Ship it', kind: 'task' } }
			})
		});
		await expect.element(page.getByText('You')).toBeVisible();
		await expect.element(page.getByText('created this task')).toBeVisible();
	});

	it('status change renders both status labels', async () => {
		render(ActivityEntry, {
			entry: makeActivity({
				action: 'item.updated',
				payload: {
					item: { seq: 1, title: 'X', kind: 'task' },
					diff: { status: { old: 'open', new: 'in_progress' } }
				}
			})
		});
		await expect.element(page.getByText('changed status')).toBeVisible();
		await expect.element(page.getByText('Open')).toBeVisible();
		await expect.element(page.getByText('In progress')).toBeVisible();
	});

	it('showItemRef toggles the #seq', async () => {
		const entry = makeActivity({
			action: 'item.created',
			payload: { item: { seq: 7, title: 'X', kind: 'task' } }
		});
		const { unmount } = render(ActivityEntry, { entry, showItemRef: true });
		await expect.element(page.getByTestId('item-ref')).toBeVisible();
		await expect.element(page.getByTestId('item-ref')).toHaveTextContent('#7');
		unmount();

		render(ActivityEntry, { entry });
		await expect.element(page.getByTestId('item-ref')).not.toBeInTheDocument();
	});

	it('multi-field update lists a sub-line per field', async () => {
		render(ActivityEntry, {
			entry: makeActivity({
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
		});
		await expect.element(page.getByText('edited this task')).toBeVisible();
		await expect.element(page.getByText('status: Open → Done')).toBeVisible();
		await expect.element(page.getByText('notes updated')).toBeVisible();
		await expect.element(page.getByText('title: "Old" → "New"')).toBeVisible();
	});

	it('a reorder still renders (entry never self-hides — surfaces filter)', async () => {
		render(ActivityEntry, {
			entry: makeActivity({
				action: 'item.updated',
				payload: {
					item: { seq: 1, title: 'X', kind: 'task' },
					diff: { position: { old: 1, new: 9 } }
				}
			})
		});
		await expect.element(page.getByText('reordered')).toBeVisible();
	});
});
