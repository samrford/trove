import { apiFetch, API_BASE_URL, ApiError, getAccessToken } from '$lib/api';
import type { Attachment } from './items';

export type UploadProgress = {
	loaded: number; // bytes sent so far
	total: number; // total bytes (0 if unknown)
};

// uploadAttachment streams a single file via multipart POST and reports upload
// progress. It uses XMLHttpRequest because fetch() can't report request-body
// (upload) progress in browsers. Returns the canonical attachment record.
export async function uploadAttachment(
	slug: string,
	seq: number,
	file: File,
	onProgress?: (progress: UploadProgress) => void,
	signal?: AbortSignal
): Promise<Attachment> {
	const path = `/v1/projects/${encodeURIComponent(slug)}/items/${seq}/attachments`;
	const token = await getAccessToken();

	if (signal?.aborted) throw new DOMException('Aborted', 'AbortError');

	return new Promise<Attachment>((resolve, reject) => {
		const xhr = new XMLHttpRequest();
		xhr.open('POST', `${API_BASE_URL}${path}`);
		if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`);

		xhr.upload.onprogress = (e) => {
			onProgress?.({ loaded: e.loaded, total: e.lengthComputable ? e.total : 0 });
		};
		xhr.onload = () => {
			if (xhr.status >= 200 && xhr.status < 300) {
				try {
					resolve(JSON.parse(xhr.responseText) as Attachment);
				} catch (e) {
					reject(new ApiError(xhr.status, `Bad JSON: ${e}`, path));
				}
			} else {
				reject(new ApiError(xhr.status, xhr.responseText || xhr.statusText, path));
			}
		};
		xhr.onerror = () => reject(new ApiError(0, 'Network error', path));
		xhr.onabort = () => reject(new DOMException('Aborted', 'AbortError'));
		signal?.addEventListener('abort', () => xhr.abort(), { once: true });

		const form = new FormData();
		form.append('file', file);
		xhr.send(form);
	});
}

export function deleteAttachment(slug: string, seq: number, id: string): Promise<void> {
	return apiFetch<void>(
		`/v1/projects/${encodeURIComponent(slug)}/items/${seq}/attachments/${encodeURIComponent(id)}`,
		{ method: 'DELETE' }
	);
}
