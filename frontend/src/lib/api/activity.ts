import { apiFetch } from '$lib/api';
import type { Activity, ActivityAction } from '$lib/activity';

// The cursor the server hands back; pass it straight back as the next page's
// position. Same shape SSE catch-up will reuse.
export type ActivityCursor = { before: string; before_id: string };

export type ActivityPage = { activity: Activity[]; next: ActivityCursor | null };

export type ListActivityOptions = {
	actions?: ActivityAction[];
	itemId?: string;
	actorId?: string;
	limit?: number;
	cursor?: ActivityCursor | null;
};

function activityUrl(slugOrID: string, opts?: ListActivityOptions): string {
	const base = `/v1/projects/${encodeURIComponent(slugOrID)}/activity`;
	const params = new URLSearchParams();
	if (opts?.actions?.length) {
		for (const a of opts.actions) params.append('action', a);
	}
	if (opts?.itemId) params.set('item', opts.itemId);
	if (opts?.actorId) params.set('actor', opts.actorId);
	if (opts?.limit) params.set('limit', String(opts.limit));
	if (opts?.cursor) {
		params.set('before', opts.cursor.before);
		params.set('before_id', opts.cursor.before_id);
	}
	const qs = params.toString();
	return qs ? `${base}?${qs}` : base;
}

// listActivity fetches one keyset page of a project's activity. Feed the
// returned `next` back via opts.cursor to page further; `next` is null at the
// end.
export function listActivity(slugOrID: string, opts?: ListActivityOptions): Promise<ActivityPage> {
	return apiFetch<ActivityPage>(activityUrl(slugOrID, opts));
}
