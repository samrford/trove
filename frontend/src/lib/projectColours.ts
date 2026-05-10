// The palette a user can pick from for a project's accent colour. Values here
// are *names*, not raw hex/oklch — the actual colour resolves to a CSS variable
// defined in `layout.css` (`--color-project-*`) so it auto-adapts between light
// and dark mode. The CSS variable prefix stays anglicised (`--color-`) because
// that's what Tailwind v4 expects; everything else uses British spelling.

export const PROJECT_COLOUR_NAMES = ['gold', 'teal', 'moss', 'rust', 'plum', 'slate'] as const;

export type ProjectColour = (typeof PROJECT_COLOUR_NAMES)[number];

export const PROJECT_COLOUR_LABELS: Record<ProjectColour, string> = {
	gold: 'Gold',
	teal: 'Teal',
	moss: 'Moss',
	rust: 'Rust',
	plum: 'Plum',
	slate: 'Slate'
};

// Returns a `var(--...)` reference for use in `style:` directives.
// Falls back to the default accent when colour is unset or unrecognised.
export function projectColourVar(colour: string | null | undefined): string {
	if (colour && (PROJECT_COLOUR_NAMES as readonly string[]).includes(colour)) {
		return `var(--color-project-${colour})`;
	}
	return 'var(--color-accent)';
}
