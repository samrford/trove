<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { getProject, deleteProject, type Project } from '$lib/api/projects';
	import { ApiError } from '$lib/api';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import AppHeader from '$lib/components/AppHeader.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import { projectColourVar } from '$lib/projectColours';
	import { ArrowLeft, Pencil, Trash2 } from '@lucide/svelte';

	let project = $state<Project | null>(null);
	let error = $state<string | null>(null);
	let deleteConfirmOpen = $state(false);
	let deleting = $state(false);

	$effect(() => {
		if (!auth.loading && !auth.user) goto('/login');
	});

	$effect(() => {
		const slug = page.params.slug;
		if (auth.user && slug && project === null) {
			getProject(slug)
				.then((res) => (project = res))
				.catch((e) => {
					error = e instanceof ApiError ? e.message : String(e);
				});
		}
	});

	async function performDelete() {
		if (!project) return;
		deleting = true;
		try {
			await deleteProject(project.slug);
			goto('/');
		} catch (e) {
			error = e instanceof ApiError ? e.message : String(e);
			deleting = false;
		}
	}
</script>

{#if !auth.loading && auth.user}
	<AppHeader />
	<main class="mx-auto max-w-4xl px-6 py-10">
		<div class="mb-6">
			<a
				href="/"
				class="inline-flex items-center gap-1.5 text-sm text-fg-muted hover:text-fg hover:underline"
			>
				<ArrowLeft class="h-4 w-4" />
				All projects
			</a>
		</div>

		{#if error}
			<div class="rounded-md border border-danger/40 bg-danger/10 p-4">
				<p class="text-sm text-danger">{error}</p>
			</div>
		{:else if project === null}
			<p class="text-sm text-fg-faint">Loading…</p>
		{:else}
			<header
				class="mb-10 border-b-4 pb-6"
				style:border-bottom-color={projectColourVar(project.colour)}
			>
				<div class="flex items-start gap-4">
					{#if project.icon}
						<span class="text-4xl leading-none">{project.icon}</span>
					{/if}
					<div class="min-w-0 flex-1">
						<div class="flex items-start justify-between gap-2">
							<h1 class="text-3xl font-semibold tracking-tight text-fg">{project.name}</h1>
							<div class="flex shrink-0 items-center gap-1">
								<a
									href={`/projects/${project.slug}/edit`}
									aria-label="Edit project"
									title="Edit"
									class="rounded-md p-2 text-fg-muted transition hover:bg-card-2 hover:text-fg"
								>
									<Pencil class="h-4 w-4" />
								</a>
								<button
									type="button"
									onclick={() => (deleteConfirmOpen = true)}
									disabled={deleting}
									aria-label="Delete project"
									title="Delete"
									class="rounded-md p-2 text-fg-muted transition hover:bg-danger/10 hover:text-danger disabled:opacity-50"
								>
									<Trash2 class="h-4 w-4" />
								</button>
							</div>
						</div>
						{#if project.description}
							<p class="mt-2 text-fg-muted">{project.description}</p>
						{/if}
					</div>
				</div>
			</header>

			<section class="rounded-lg border border-line bg-card p-12 text-center">
				<p class="text-fg-muted">No items yet.</p>
				<p class="mt-2 text-sm text-fg-faint">
					Brainstorms, tasks, and bugs will land here once that bit is built.
				</p>
			</section>

			<ConfirmDialog
				bind:open={deleteConfirmOpen}
				title={`Delete "${project.name}"?`}
				message="This is permanent. The project and all of its items, tags, attachments, and activity will be removed."
				confirmLabel="Delete project"
				cancelLabel="Keep it"
				destructive={true}
				onConfirm={performDelete}
			/>
		{/if}
	</main>
{/if}
