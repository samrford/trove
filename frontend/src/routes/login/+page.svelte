<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { goto } from '$app/navigation';
	import GoogleIcon from '$lib/components/GoogleIcon.svelte';
	import GithubIcon from '$lib/components/GithubIcon.svelte';
	import DiscordIcon from '$lib/components/DiscordIcon.svelte';

	let email = $state('');
	let password = $state('');
	let mode = $state<'signin' | 'signup'>('signin');
	let error = $state<string | null>(null);
	let loading = $state(false);

	$effect(() => {
		if (auth.user) goto('/');
	});

	async function handleProvider(provider: 'google' | 'github' | 'discord') {
		error = null;
		loading = true;
		const { error: err } = await auth.signInWithProvider(provider);
		if (err) error = err.message;
		loading = false;
	}

	async function handlePassword(e: SubmitEvent) {
		e.preventDefault();
		error = null;
		loading = true;
		const { error: err } =
			mode === 'signin'
				? await auth.signInWithPassword(email, password)
				: await auth.signUpWithPassword(email, password);
		if (err) error = err.message;
		loading = false;
	}
</script>

<main class="mx-auto flex min-h-screen max-w-md items-center px-6 py-10">
	<div class="w-full">
		<img src="/logolarge.png" alt="Trove" class="mx-auto mb-6 w-full max-w-xs" />
		<p class="mb-8 text-center text-sm text-fg-muted">
			A cozy little place to keep your projects, ideas, and the things you're chasing.
		</p>

		<div class="mb-5 flex flex-col gap-2">
			<button
				class="flex items-center justify-center gap-2 rounded-md border border-line px-4 py-2 text-sm font-medium text-fg transition hover:bg-card-2 disabled:opacity-50"
				disabled={loading}
				onclick={() => handleProvider('google')}
			>
				<GoogleIcon class="h-4 w-4" />
				Sign in with Google
			</button>
			<button
				class="flex items-center justify-center gap-2 rounded-md border border-line px-4 py-2 text-sm font-medium text-fg transition hover:bg-card-2 disabled:opacity-50"
				disabled={loading}
				onclick={() => handleProvider('github')}
			>
				<GithubIcon class="h-4 w-4" />
				Sign in with GitHub
			</button>
			<button
				class="flex items-center justify-center gap-2 rounded-md border border-line px-4 py-2 text-sm font-medium text-fg transition hover:bg-card-2 disabled:opacity-50"
				disabled={loading}
				onclick={() => handleProvider('discord')}
			>
				<DiscordIcon class="h-4 w-4" />
				Sign in with Discord
			</button>
		</div>

		<div class="mb-3 text-center text-xs tracking-wider text-fg-faint uppercase">or</div>

		<form onsubmit={handlePassword} class="flex flex-col gap-2">
			<input
				type="email"
				bind:value={email}
				placeholder="email"
				required
				class="rounded-md border border-line bg-card px-3 py-2 text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
			/>
			<input
				type="password"
				bind:value={password}
				placeholder="password"
				required
				class="rounded-md border border-line bg-card px-3 py-2 text-sm text-fg placeholder:text-fg-faint focus:border-accent focus:outline-none"
			/>
			<button
				type="submit"
				disabled={loading}
				class="rounded-md bg-accent px-4 py-2 text-sm font-medium text-on-accent transition hover:bg-accent-hover disabled:opacity-50"
			>
				{mode === 'signin' ? 'Sign in' : 'Sign up'}
			</button>
			<button
				type="button"
				class="text-xs text-fg-faint hover:text-fg-muted hover:underline"
				onclick={() => (mode = mode === 'signin' ? 'signup' : 'signin')}
			>
				{mode === 'signin' ? 'or sign up' : 'or sign in'}
			</button>
		</form>

		{#if error}
			<p class="mt-4 text-sm text-danger">{error}</p>
		{/if}
	</div>
</main>
