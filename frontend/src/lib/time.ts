// Compact relative-time formatting. Returns short forms like "2d", "5m",
// "1y" — designed for narrow columns in item rows / activity feeds. Pass a
// full ISO timestamp; on hover, the consumer should show the absolute time.

export function relativeTime(iso: string): string {
	const diffMs = Date.now() - new Date(iso).getTime();
	const sec = Math.floor(diffMs / 1000);
	if (sec < 60) return 'just now';
	const min = Math.floor(sec / 60);
	if (min < 60) return `${min}m`;
	const hr = Math.floor(min / 60);
	if (hr < 24) return `${hr}h`;
	const day = Math.floor(hr / 24);
	if (day < 30) return `${day}d`;
	const mo = Math.floor(day / 30);
	if (mo < 12) return `${mo}mo`;
	return `${Math.floor(mo / 12)}y`;
}
