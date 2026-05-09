import { supabase } from '$lib/supabase';
import { PUBLIC_API_BASE_URL } from '$env/static/public';

export const API_BASE_URL = PUBLIC_API_BASE_URL ?? '';

async function getAccessToken(): Promise<string | null> {
	const { data } = await supabase.auth.getSession();
	return data.session?.access_token ?? null;
}

async function authHeaders(): Promise<Record<string, string>> {
	const token = await getAccessToken();
	return token ? { Authorization: `Bearer ${token}` } : {};
}

export async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
	const auth = await authHeaders();

	const response = await fetch(`${API_BASE_URL}${path}`, {
		headers: {
			'Content-Type': 'application/json',
			...auth,
			...options?.headers
		},
		...options
	});

	if (!response.ok) {
		const errorText = await response.text().catch(() => 'Unknown error');
		throw new ApiError(response.status, errorText, path);
	}

	if (response.status === 204) {
		return undefined as T;
	}

	return response.json();
}

export class ApiError extends Error {
	status: number;
	body: string;
	endpoint: string;

	constructor(status: number, body: string, endpoint: string) {
		super(`API ${status} on ${endpoint}: ${body}`);
		this.name = 'ApiError';
		this.status = status;
		this.body = body;
		this.endpoint = endpoint;
	}
}
