<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { goto } from '$app/navigation';

	let email = $state('');
	let password = $state('');
	let mode = $state<'signin' | 'signup'>('signin');
	let error = $state<string | null>(null);
	let loading = $state(false);

	// If already signed in, bounce home.
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

<div class="mx-auto max-w-md p-8">
	<h1 class="mb-2 text-3xl font-bold">Trove</h1>
	<p class="mb-8 text-gray-600">
		A cozy little place to keep your projects, ideas, and the things you're chasing.
	</p>

	<div class="mb-6 flex flex-col gap-2">
		<button
			class="rounded border px-4 py-2 hover:bg-gray-50 disabled:opacity-50"
			disabled={loading}
			onclick={() => handleProvider('google')}>Sign in with Google</button
		>
		<button
			class="rounded border px-4 py-2 hover:bg-gray-50 disabled:opacity-50"
			disabled={loading}
			onclick={() => handleProvider('github')}>Sign in with GitHub</button
		>
		<button
			class="rounded border px-4 py-2 hover:bg-gray-50 disabled:opacity-50"
			disabled={loading}
			onclick={() => handleProvider('discord')}>Sign in with Discord</button
		>
	</div>

	<div class="mb-4 text-center text-sm text-gray-500">or with email + password</div>

	<form onsubmit={handlePassword} class="flex flex-col gap-2">
		<input
			type="email"
			bind:value={email}
			placeholder="email"
			required
			class="rounded border px-3 py-2"
		/>
		<input
			type="password"
			bind:value={password}
			placeholder="password"
			required
			class="rounded border px-3 py-2"
		/>
		<button
			type="submit"
			disabled={loading}
			class="rounded bg-black px-4 py-2 text-white hover:bg-gray-800 disabled:opacity-50"
		>
			{mode === 'signin' ? 'Sign in' : 'Sign up'}
		</button>
		<button
			type="button"
			class="text-sm text-gray-500 hover:underline"
			onclick={() => (mode = mode === 'signin' ? 'signup' : 'signin')}
		>
			{mode === 'signin' ? 'or sign up' : 'or sign in'}
		</button>
	</form>

	{#if error}
		<p class="mt-4 text-sm text-red-600">{error}</p>
	{/if}
</div>
