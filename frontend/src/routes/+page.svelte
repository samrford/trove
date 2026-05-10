<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { listProjects, type Project } from '$lib/api/projects';
	import { ApiError } from '$lib/api';
	import { goto } from '$app/navigation';
	import AppHeader from '$lib/components/AppHeader.svelte';
	import { projectColourVar } from '$lib/projectColours';
	import { Plus } from '@lucide/svelte';

	let projects = $state<Project[] | null>(null);
	let error = $state<string | null>(null);

	$effect(() => {
		if (!auth.loading && !auth.user) goto('/login');
	});

	$effect(() => {
		if (auth.user && projects === null) {
			listProjects()
				.then((res) => (projects = res))
				.catch((e) => {
					error = e instanceof ApiError ? e.message : String(e);
				});
		}
	});

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}
</script>

{#if !auth.loading && auth.user}
	<AppHeader />
	<main class="mx-auto max-w-5xl px-6 py-10">
		<div class="mb-8 flex items-center justify-between">
			<div>
				<h1 class="text-2xl font-semibold tracking-tight text-fg">Your trove</h1>
				<p class="mt-1 text-sm text-fg-muted">Projects you own or have been invited to.</p>
			</div>
			<a
				href="/projects/new"
				class="inline-flex items-center gap-1.5 rounded-md bg-accent px-4 py-2 text-sm font-medium text-on-accent transition hover:bg-accent-hover"
			>
				<Plus class="h-4 w-4" />
				New project
			</a>
		</div>

		{#if error}
			<div class="mb-4 rounded-md border border-danger/40 bg-danger/10 p-4">
				<p class="text-sm text-danger">{error}</p>
			</div>
		{/if}

		{#if projects === null && !error}
			<p class="text-sm text-fg-faint">Loading…</p>
		{:else if projects && projects.length === 0}
			<div class="rounded-lg border border-line bg-card p-12 text-center">
				<p class="text-fg-muted">Your trove is empty — toss something in.</p>
				<a
					href="/projects/new"
					class="mt-4 inline-block text-sm text-accent hover:text-accent-hover hover:underline"
				>
					Create your first project →
				</a>
			</div>
		{:else if projects}
			<ul class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
				{#each projects as project (project.id)}
					<li>
						<a
							href={`/projects/${project.slug}`}
							class="relative block overflow-hidden rounded-lg border border-line bg-card p-5 pl-6 transition hover:border-line-strong"
						>
							<span
								aria-hidden="true"
								class="absolute top-0 bottom-0 left-0 w-1.5"
								style:background-color={projectColourVar(project.colour)}
							></span>
							<div class="flex items-start gap-3">
								{#if project.icon}
									<span class="text-2xl leading-none">{project.icon}</span>
								{/if}
								<div class="min-w-0 flex-1">
									<h2 class="truncate font-semibold text-fg">{project.name}</h2>
									{#if project.description}
										<p class="mt-1 line-clamp-2 text-sm text-fg-muted">{project.description}</p>
									{/if}
									<p class="mt-3 text-xs text-fg-faint">
										Updated {formatDate(project.updated_at)}
									</p>
								</div>
							</div>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</main>
{/if}
