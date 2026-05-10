<script lang="ts">
	import { theme, type ThemePreference } from '$lib/theme.svelte';
	import { Sun, Monitor, Moon } from '@lucide/svelte';

	const options: { value: ThemePreference; icon: typeof Sun; label: string }[] = [
		{ value: 'light', icon: Sun, label: 'Light' },
		{ value: 'system', icon: Monitor, label: 'System' },
		{ value: 'dark', icon: Moon, label: 'Dark' }
	];
</script>

<div
	class="flex rounded-md border border-line p-0.5"
	role="radiogroup"
	aria-label="Theme preference"
>
	{#each options as option (option.value)}
		{@const Icon = option.icon}
		<button
			type="button"
			role="radio"
			aria-checked={theme.preference === option.value}
			aria-label={option.label}
			title={option.label}
			onclick={() => theme.setPreference(option.value)}
			class="rounded p-1.5 transition {theme.preference === option.value
				? 'bg-card-2 text-fg'
				: 'text-fg-muted hover:text-fg'}"
		>
			<Icon class="h-4 w-4" />
		</button>
	{/each}
</div>
