import { marked } from 'marked';
import DOMPurify from 'dompurify';

marked.setOptions({ gfm: true, breaks: true });

export function renderMarkdown(input: string): string {
	if (!input) return '';
	return DOMPurify.sanitize(marked.parse(input) as string);
}

// Svelte action: parse + sanitize markdown and mount the result inside the
// host element.
export function markdownBody(node: HTMLElement, source: string) {
	node.innerHTML = renderMarkdown(source);
	return {
		update(next: string) {
			node.innerHTML = renderMarkdown(next);
		}
	};
}
