// Curated emoji palette for project icons. Grouped loosely by vibe so the
// picker has a discoverable shape: work / making / digital / life / atmosphere.
// 40 picks chosen to fit "what's this project for?" — broad enough to cover most
// personal-tracker projects without becoming a full Unicode emoji keyboard.

export const PROJECT_EMOJIS = [
	// work & study
	'💼',
	'📋',
	'📝',
	'📊',
	'📚',
	'🎓',
	'💡',
	'🎯',
	// making & tinkering
	'🛠️',
	'🔨',
	'⚙️',
	'🧪',
	'🔬',
	'🧰',
	'⚡',
	'🚀',
	// digital
	'💻',
	'⌨️',
	'🖱️',
	'📱',
	'🌐',
	'🤖',
	'🎮',
	'📷',
	// life & home
	'🏠',
	'🍝',
	'🍰',
	'🛒',
	'🌱',
	'🐾',
	'🚲',
	'✈️',
	// atmosphere
	'✨',
	'💎',
	'🪙',
	'📦',
	'❤️',
	'🌙',
	'⭐',
	'🔥'
] as const;

export type ProjectEmoji = (typeof PROJECT_EMOJIS)[number];
