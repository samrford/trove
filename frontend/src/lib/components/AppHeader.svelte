<script lang="ts">
	import { auth, userDisplayName, userAvatarUrl, userInitial } from '$lib/auth.svelte';
	import { goto } from '$app/navigation';
	import { LogOut } from '@lucide/svelte';
	import ThemeSwitcher from '$lib/components/ThemeSwitcher.svelte';
	import AnimationToggle from '$lib/components/AnimationToggle.svelte';

	let displayName = $derived(userDisplayName(auth.user));
	let avatarUrl = $derived(userAvatarUrl(auth.user));
	let initial = $derived(userInitial(auth.user));

	async function handleSignOut() {
		await auth.signOut();
		goto('/login');
	}
</script>

<header class="border-b border-line">
	<div class="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
		<a href="/" class="flex items-center gap-2.5">
			<img src="/logosmall.png" alt="" class="h-8 w-8 rounded-md" />
			<span class="text-lg font-semibold tracking-tight text-fg">Trove</span>
		</a>

		{#if auth.user}
			<div class="flex items-center gap-3">
				<ThemeSwitcher />
				<AnimationToggle />
				<div class="flex items-center gap-2">
					{#if avatarUrl}
						<img
							src={avatarUrl}
							alt=""
							referrerpolicy="no-referrer"
							class="h-7 w-7 rounded-full border border-line object-cover"
						/>
					{:else}
						<div
							class="flex h-7 w-7 items-center justify-center rounded-full border border-line bg-card-2 text-xs font-medium text-fg-muted"
							aria-hidden="true"
						>
							{initial}
						</div>
					{/if}
					<span class="hidden text-sm text-fg-muted sm:inline">{displayName}</span>
				</div>
				<button
					type="button"
					onclick={handleSignOut}
					aria-label="Sign out"
					title="Sign out"
					class="rounded-md p-2 text-fg-muted transition hover:bg-card-2 hover:text-fg"
				>
					<LogOut class="h-4 w-4" />
				</button>
			</div>
		{/if}
	</div>
</header>
