export const TAG_COLOUR_NAMES = [
	'ruby',
	'emerald',
	'sapphire',
	'amethyst',
	'citrine',
	'coral',
	'turquoise',
	'indigo'
] as const;

export type TagColour = (typeof TAG_COLOUR_NAMES)[number];

export const TAG_COLOUR_LABELS: Record<TagColour, string> = {
	ruby: 'Ruby',
	emerald: 'Emerald',
	sapphire: 'Sapphire',
	amethyst: 'Amethyst',
	citrine: 'Citrine',
	coral: 'Coral',
	turquoise: 'Turquoise',
	indigo: 'Indigo'
};

export function tagColourVar(colour: string | null | undefined): string {
	if (colour && (TAG_COLOUR_NAMES as readonly string[]).includes(colour)) {
		return `var(--color-tag-${colour})`;
	}
	return 'var(--color-fg-muted)';
}

// Picks a colour deterministically from a tag name. Used when a tag is created
// inline from the combobox (no colour picker shown — we just pick one).
export function tagColourFromName(name: string): TagColour {
	let hash = 0;
	for (let i = 0; i < name.length; i++) {
		hash = (hash * 31 + name.charCodeAt(i)) >>> 0;
	}
	return TAG_COLOUR_NAMES[hash % TAG_COLOUR_NAMES.length];
}
