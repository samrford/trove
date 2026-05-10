import { apiFetch } from '$lib/api';

export type Project = {
	id: string;
	slug: string;
	name: string;
	description: string | null;
	colour: string | null;
	icon: string | null;
	owner_id: string;
	archived_at: string | null;
	created_at: string;
	updated_at: string;
};

export type ProjectInput = {
	name: string;
	slug?: string;
	description?: string | null;
	colour?: string | null;
	icon?: string | null;
};

export function listProjects(): Promise<Project[]> {
	return apiFetch<Project[]>('/v1/projects');
}

export function getProject(slugOrID: string): Promise<Project> {
	return apiFetch<Project>(`/v1/projects/${encodeURIComponent(slugOrID)}`);
}

export function createProject(input: ProjectInput): Promise<Project> {
	return apiFetch<Project>('/v1/projects', {
		method: 'POST',
		body: JSON.stringify(input)
	});
}

// PATCH semantics: send the full replacement set every time. `null` for
// description/color/icon explicitly clears the field. `slug` is optional —
// omit or pass "" to leave it unchanged.
export type ProjectUpdate = {
	name: string;
	slug?: string;
	description: string | null;
	colour: string | null;
	icon: string | null;
};

export type SlugCheck = { available: boolean; reason?: 'invalid' };

export function checkSlug(slug: string, signal?: AbortSignal): Promise<SlugCheck> {
	return apiFetch<SlugCheck>(`/v1/projects/check-slug?slug=${encodeURIComponent(slug)}`, {
		signal
	});
}

export function updateProject(slugOrID: string, input: ProjectUpdate): Promise<Project> {
	return apiFetch<Project>(`/v1/projects/${encodeURIComponent(slugOrID)}`, {
		method: 'PATCH',
		body: JSON.stringify(input)
	});
}

export function deleteProject(slugOrID: string): Promise<void> {
	return apiFetch<void>(`/v1/projects/${encodeURIComponent(slugOrID)}`, {
		method: 'DELETE'
	});
}
