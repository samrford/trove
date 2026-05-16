import { apiFetch } from '$lib/api';
import type { Tag } from './tags';

export type ItemKind = 'brainstorm' | 'task';
export type ItemStatus = 'open' | 'in_progress' | 'done' | 'archived';

export const ITEM_KINDS: ItemKind[] = ['brainstorm', 'task'];
export const ITEM_STATUSES: ItemStatus[] = ['open', 'in_progress', 'done', 'archived'];

export type Item = {
	id: string;
	project_id: string;
	sequence: number;
	kind: ItemKind;
	status: ItemStatus;
	title: string;
	body: string | null;
	position: number;
	creator_id: string;
	created_at: string;
	updated_at: string;
	tags: Tag[];
	attachments: Attachment[];
};

export type AttachmentSource = 'upload' | 'google_photos';

export type Attachment = {
	id: string;
	project_id: string;
	item_id: string | null;
	filename: string;
	content_type: string;
	size_bytes: number;
	source: AttachmentSource;
	uploader_id: string;
	created_at: string;
	url: string;
};

export type ItemInput = {
	kind: ItemKind;
	title: string;
	body?: string | null;
};

// PATCH semantics: only the fields you pass get updated. Omit to leave alone.
// To clear `body`, pass an empty string (server treats it as NULL).
export type ItemUpdate = {
	title?: string;
	body?: string | null;
	kind?: ItemKind;
	status?: ItemStatus;
	position?: number;
};

export type TagFilterMode = 'and' | 'or';

export type ListItemsOptions = {
	kind?: ItemKind;
	status?: ItemStatus;
	tags?: string[];
	tagMode?: TagFilterMode;
};

function projectItemsUrl(slugOrID: string, options?: ListItemsOptions): string {
	const base = `/v1/projects/${encodeURIComponent(slugOrID)}/items`;
	if (!options?.kind && !options?.status && !options?.tags?.length) return base;
	const params = new URLSearchParams();
	if (options.kind) params.set('kind', options.kind);
	if (options.status) params.set('status', options.status);
	if (options.tags?.length) {
		for (const slug of options.tags) params.append('tag', slug);
		if (options.tagMode) params.set('tag_mode', options.tagMode);
	}
	return `${base}?${params}`;
}

export function listItems(slugOrID: string, options?: ListItemsOptions): Promise<Item[]> {
	return apiFetch<Item[]>(projectItemsUrl(slugOrID, options));
}

export function getItem(slugOrID: string, sequence: number): Promise<Item> {
	return apiFetch<Item>(`/v1/projects/${encodeURIComponent(slugOrID)}/items/${sequence}`);
}

export function createItem(slugOrID: string, input: ItemInput): Promise<Item> {
	return apiFetch<Item>(`/v1/projects/${encodeURIComponent(slugOrID)}/items`, {
		method: 'POST',
		body: JSON.stringify(input)
	});
}

export function updateItem(slugOrID: string, sequence: number, input: ItemUpdate): Promise<Item> {
	return apiFetch<Item>(`/v1/projects/${encodeURIComponent(slugOrID)}/items/${sequence}`, {
		method: 'PATCH',
		body: JSON.stringify(input)
	});
}

export function deleteItem(slugOrID: string, sequence: number): Promise<void> {
	return apiFetch<void>(`/v1/projects/${encodeURIComponent(slugOrID)}/items/${sequence}`, {
		method: 'DELETE'
	});
}

// Attach a tag to an item. Accepts an existing tag (by id or slug) or a name
// to find-or-create. Returns the (possibly newly-created) tag.
export type AttachTagInput =
	| { tag_id: string }
	| { tag_slug: string }
	| { name: string; colour?: string | null; icon?: string | null };

export function attachTagToItem(
	slugOrID: string,
	sequence: number,
	input: AttachTagInput
): Promise<Tag> {
	return apiFetch<Tag>(`/v1/projects/${encodeURIComponent(slugOrID)}/items/${sequence}/tags`, {
		method: 'POST',
		body: JSON.stringify(input)
	});
}

export function detachTagFromItem(
	slugOrID: string,
	sequence: number,
	tagSlug: string
): Promise<void> {
	return apiFetch<void>(
		`/v1/projects/${encodeURIComponent(slugOrID)}/items/${sequence}/tags/${encodeURIComponent(tagSlug)}`,
		{ method: 'DELETE' }
	);
}
