// API-client test (server/node project). Mocks $lib/api so we assert the URL
// listActivity builds + that it passes the response through — without loading
// the real apiFetch (supabase/env).

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { apiFetch } from '$lib/api';
import { listActivity } from './activity';
import type { ActivityPage } from './activity';

vi.mock('$lib/api', () => ({ apiFetch: vi.fn() }));

const mockFetch = vi.mocked(apiFetch);

function calledPath(): string {
	return mockFetch.mock.calls[0][0] as string;
}

describe('listActivity', () => {
	beforeEach(() => {
		mockFetch.mockReset();
		mockFetch.mockResolvedValue({ activity: [], next: null } satisfies ActivityPage);
	});

	it('bare path when no options', async () => {
		await listActivity('garden');
		expect(calledPath()).toBe('/v1/projects/garden/activity');
	});

	it('encodes the project identifier', async () => {
		await listActivity('a/b space');
		expect(calledPath()).toBe('/v1/projects/a%2Fb%20space/activity');
	});

	it('repeats action= per action and sets item/actor/limit', async () => {
		await listActivity('garden', {
			actions: ['item.created', 'item.updated'],
			itemId: 'i1',
			actorId: 'u1',
			limit: 20
		});
		const u = new URL('http://x' + calledPath());
		expect(u.searchParams.getAll('action')).toEqual(['item.created', 'item.updated']);
		expect(u.searchParams.get('item')).toBe('i1');
		expect(u.searchParams.get('actor')).toBe('u1');
		expect(u.searchParams.get('limit')).toBe('20');
	});

	it('threads the cursor as before/before_id', async () => {
		await listActivity('garden', {
			cursor: { before: '2026-05-17T12:00:00.000Z', before_id: 'act-9' }
		});
		const u = new URL('http://x' + calledPath());
		expect(u.searchParams.get('before')).toBe('2026-05-17T12:00:00.000Z');
		expect(u.searchParams.get('before_id')).toBe('act-9');
	});

	it('passes the response through unchanged', async () => {
		const page: ActivityPage = {
			activity: [],
			next: { before: 't', before_id: 'id' }
		};
		mockFetch.mockResolvedValue(page);
		await expect(listActivity('garden')).resolves.toBe(page);
	});
});
