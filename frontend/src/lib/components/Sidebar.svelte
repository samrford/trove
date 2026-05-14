<script lang="ts">
	import { auth, userDisplayName, userAvatarUrl, userInitial } from '$lib/auth.svelte';
	import { APP_VERSION } from '$lib/version';
	import { sidebar } from '$lib/sidebar.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { LogOut, Menu, X, House, Tags, ChevronLeft, ChevronRight } from '@lucide/svelte';
	import ThemeSwitcher from './ThemeSwitcher.svelte';
	import AnimationToggle from './AnimationToggle.svelte';

	let displayName = $derived(userDisplayName(auth.user));
	let avatarUrl = $derived(userAvatarUrl(auth.user));
	let initial = $derived(userInitial(auth.user));

	let isMobileOpen = $state(false);

	// The mobile drawer always shows the expanded layout, regardless of the
	// desktop collapse preference. Only collapse styling kicks in at lg+.
	let collapsed = $derived(sidebar.collapsed);

	// Close mobile drawer on navigation
	$effect(() => {
		// Reactivity-only access — close the drawer whenever the pathname changes.
		void page.url.pathname;
		isMobileOpen = false;
	});

	// Lock body scroll when mobile drawer is open
	$effect(() => {
		if (!isMobileOpen) return;
		const prev = document.body.style.overflow;
		document.body.style.overflow = 'hidden';
		return () => {
			document.body.style.overflow = prev;
		};
	});

	async function handleSignOut() {
		await auth.signOut();
		goto('/login');
	}

	const navItems = [
		{
			name: 'Projects',
			href: '/',
			icon: House,
			matches: (path: string) => path === '/' || path.startsWith('/projects')
		},
		{ name: 'Tags', href: '/tags', icon: Tags, matches: (path: string) => path.startsWith('/tags') }
	];
</script>

<!-- Mobile header bar (provides space for hamburger button, sits above page content) -->
<header class="flex h-14 items-center gap-3 border-b border-line bg-card px-3 lg:hidden">
	<button
		type="button"
		onclick={() => (isMobileOpen = true)}
		aria-label="Open menu"
		class="rounded-md p-2 text-fg-muted transition hover:bg-card-2 hover:text-fg"
	>
		<Menu class="h-5 w-5" />
	</button>
	<a href="/" class="flex items-center gap-2 text-fg">
		<img src="/logosmall.png" alt="" class="h-7 w-7 rounded-md" />
		<span class="text-base font-semibold tracking-tight">Trove</span>
	</a>
</header>

<!-- Mobile overlay -->
{#if isMobileOpen}
	<button
		type="button"
		aria-label="Close menu"
		onclick={() => (isMobileOpen = false)}
		class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm lg:hidden"
	></button>
{/if}

<aside
	aria-label="Primary"
	class="fixed top-0 left-0 z-50 flex h-screen w-60 flex-col border-r border-line bg-card transition-[transform,width] duration-200 lg:translate-x-0"
	class:translate-x-0={isMobileOpen}
	class:-translate-x-full={!isMobileOpen}
	class:lg:w-16={collapsed}
>
	<!-- Brand + collapse toggle -->
	<div class="flex items-center justify-between gap-2 px-3 py-4 lg:px-4">
		<a href="/" class="flex min-w-0 items-center gap-2.5" title="Trove">
			<img src="/logosmall.png" alt="" class="h-8 w-8 shrink-0 rounded-md" />
			<span
				class="truncate text-lg font-semibold tracking-tight text-fg"
				class:lg:hidden={collapsed}>Trove</span
			>
		</a>
		<!-- Mobile: close drawer -->
		<button
			type="button"
			onclick={() => (isMobileOpen = false)}
			aria-label="Close menu"
			class="rounded-md p-1.5 text-fg-muted transition hover:bg-card-2 hover:text-fg lg:hidden"
		>
			<X class="h-4 w-4" />
		</button>
		<!-- Desktop: collapse/expand -->
		<button
			type="button"
			onclick={() => sidebar.toggle()}
			aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
			title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
			class="hidden shrink-0 rounded-md p-1.5 text-fg-muted transition hover:bg-card-2 hover:text-fg lg:inline-flex"
		>
			{#if collapsed}
				<ChevronRight class="h-4 w-4" />
			{:else}
				<ChevronLeft class="h-4 w-4" />
			{/if}
		</button>
	</div>

	<!-- Nav -->
	<nav class="flex flex-1 flex-col gap-1 px-3">
		{#each navItems as item (item.href)}
			{@const Icon = item.icon}
			{@const active = item.matches(page.url.pathname)}
			<a
				href={item.href}
				title={item.name}
				class="flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition"
				class:lg:justify-center={collapsed}
				class:lg:px-2={collapsed}
				class:bg-card-2={active}
				class:text-fg={active}
				class:text-fg-muted={!active}
				class:hover:bg-card-2={!active}
				class:hover:text-fg={!active}
				aria-current={active ? 'page' : undefined}
			>
				<Icon class="h-4 w-4 shrink-0" />
				<span class:lg:hidden={collapsed}>{item.name}</span>
			</a>
		{/each}
	</nav>

	<!-- Footer: theme/animation, user, version. Hide most of it when collapsed. -->
	<div class="flex flex-col gap-3 border-t border-line px-3 py-3">
		<div class="flex items-center justify-between gap-2 px-1" class:lg:hidden={collapsed}>
			<ThemeSwitcher />
			<AnimationToggle />
		</div>

		{#if auth.user}
			<div
				class="flex items-center gap-2 rounded-md px-1 py-1"
				class:lg:justify-center={collapsed}
				class:lg:px-0={collapsed}
			>
				{#if avatarUrl}
					<img
						src={avatarUrl}
						alt=""
						referrerpolicy="no-referrer"
						title={displayName}
						class="h-8 w-8 shrink-0 rounded-full border border-line object-cover"
					/>
				{:else}
					<div
						title={displayName}
						class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-line bg-card-2 text-xs font-medium text-fg-muted"
						aria-hidden="true"
					>
						{initial}
					</div>
				{/if}
				<span class="min-w-0 flex-1 truncate text-sm text-fg-muted" class:lg:hidden={collapsed}
					>{displayName}</span
				>
				<button
					type="button"
					onclick={handleSignOut}
					aria-label="Sign out"
					title="Sign out"
					class="shrink-0 rounded-md p-1.5 text-fg-muted transition hover:bg-card-2 hover:text-danger"
					class:lg:hidden={collapsed}
				>
					<LogOut class="h-4 w-4" />
				</button>
			</div>
		{/if}

		<div class="flex justify-center pb-0.5" class:lg:hidden={collapsed}>
			<span class="font-mono text-[10px] text-fg-faint" title="App version">v{APP_VERSION}</span>
		</div>
	</div>
</aside>
