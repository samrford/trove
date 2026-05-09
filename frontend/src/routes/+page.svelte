<script lang="ts">
	import { auth } from '$lib/auth.svelte'
	import { apiFetch, ApiError } from '$lib/api'
	import { goto } from '$app/navigation'

	type MeResponse = { id: string; email: string }

	let me = $state<MeResponse | null>(null)
	let error = $state<string | null>(null)

	// Once auth has resolved, redirect to /login if there's no user.
	$effect(() => {
		if (!auth.loading && !auth.user) goto('/login')
	})

	// When we have a user, hit the Go backend's /v1/me to confirm the
	// end-to-end auth flow (browser → Bearer token → Go OIDC verify → UpsertUser).
	$effect(() => {
		if (auth.user) {
			apiFetch<MeResponse>('/v1/me')
				.then((res) => (me = res))
				.catch((e) => {
					error = e instanceof ApiError ? e.message : String(e)
				})
		}
	})

	async function handleSignOut() {
		await auth.signOut()
		goto('/login')
	}
</script>

<div class="mx-auto max-w-2xl p-8">
	<h1 class="mb-2 text-3xl font-bold">Trove</h1>
	<p class="mb-8 text-gray-600">Welcome to Trove!</p>

	{#if auth.loading}
		<p class="text-gray-500">Loading…</p>
	{:else if !auth.user}
		<p class="text-gray-500">Redirecting to /login…</p>
	{:else}
		<div class="mb-4 rounded border p-6">
			<p class="mb-1 text-sm text-gray-500">Logged in as</p>
			<p class="text-lg">{auth.user.email}</p>
		</div>

		{#if me}
			<div class="mb-4 rounded border p-6">
				<p class="mb-2 text-sm text-gray-500">/v1/me from the Go backend</p>
				<pre class="text-sm">{JSON.stringify(me, null, 2)}</pre>
			</div>
		{:else if error}
			<div class="mb-4 rounded border border-red-300 bg-red-50 p-6">
				<p class="mb-2 text-sm text-red-700">Error from backend</p>
				<pre class="text-sm">{error}</pre>
			</div>
		{:else}
			<p class="text-gray-500">Calling /v1/me…</p>
		{/if}

		<button onclick={handleSignOut} class="mt-4 text-sm text-gray-500 hover:underline">
			Sign out
		</button>
	{/if}
</div>
