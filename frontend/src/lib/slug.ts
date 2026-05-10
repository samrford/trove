// slugify mirrors the backend's generateSlug (backend/internal/data/projects.go):
// lowercase alphanumerics, runs of non-alphanumeric chars collapse to a single
// dash, leading dashes never appear, trailing dashes are trimmed.

export function slugify(input: string): string {
	let out = '';
	let prevDash = false;
	for (const ch of input.toLowerCase()) {
		if ((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
			out += ch;
			prevDash = false;
		} else if (!prevDash && out.length > 0) {
			out += '-';
			prevDash = true;
		}
	}
	return out.replace(/-+$/, '');
}

// A slug is valid iff it matches the canonical form slugify would produce
// from itself — non-empty, lowercase alphanumerics, single dashes between.
export function isValidSlug(slug: string): boolean {
	return slug.length > 0 && slug === slugify(slug);
}
