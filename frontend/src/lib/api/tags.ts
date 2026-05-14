import { apiFetch } from '$lib/api';
import type { Item } from './items';
import type { SlugCheck } from './projects';

export type Tag = {
	id: string;
	slug: string;
	name: string;
	description: string | null;
	icon: string | null;
	colour: string | null;
	user_id: string | null;
	group_id: string | null;
	archived_at: string | null;
	created_at: string;
	updated_at: string;
};

// Returned by list endpoints — Tag plus the usage count (global for /v1/tags,
// per-project for /v1/projects/:slug/tags) and the most-recent application
// timestamp (null if never applied).
export type TagWithCount = Tag & {
	item_count: number;
	last_used_at: string | null;
};

export type TagInput = {
	name: string;
	slug?: string;
	description?: string | null;
	colour?: string | null;
	icon?: string | null;
};

export type TagUpdate = {
	name: string;
	slug?: string;
	description: string | null;
	colour: string | null;
	icon: string | null;
};

export function listTags(): Promise<TagWithCount[]> {
	return apiFetch<TagWithCount[]>('/v1/tags');
}

export function getTag(slugOrID: string): Promise<Tag> {
	return apiFetch<Tag>(`/v1/tags/${encodeURIComponent(slugOrID)}`);
}

export function createTag(input: TagInput): Promise<Tag> {
	return apiFetch<Tag>('/v1/tags', {
		method: 'POST',
		body: JSON.stringify(input)
	});
}

export function updateTag(slugOrID: string, input: TagUpdate): Promise<Tag> {
	return apiFetch<Tag>(`/v1/tags/${encodeURIComponent(slugOrID)}`, {
		method: 'PATCH',
		body: JSON.stringify(input)
	});
}

export function deleteTag(slugOrID: string): Promise<void> {
	return apiFetch<void>(`/v1/tags/${encodeURIComponent(slugOrID)}`, {
		method: 'DELETE'
	});
}

export function checkTagSlug(slug: string, signal?: AbortSignal): Promise<SlugCheck> {
	return apiFetch<SlugCheck>(`/v1/tags/check-slug?slug=${encodeURIComponent(slug)}`, {
		signal
	});
}

export function listItemsForTag(slugOrID: string): Promise<Item[]> {
	return apiFetch<Item[]>(`/v1/tags/${encodeURIComponent(slugOrID)}/items`);
}

export function listTagsForProject(projectSlugOrID: string): Promise<TagWithCount[]> {
	return apiFetch<TagWithCount[]>(`/v1/projects/${encodeURIComponent(projectSlugOrID)}/tags`);
}
