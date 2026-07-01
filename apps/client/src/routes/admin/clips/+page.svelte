<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Clip } from '$lib/api';
	import Icon from '$lib/components/Icon.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Card from '$lib/components/ui/Card.svelte';

	let clips = $state<Clip[]>([]);
	let loading = $state(true);
	let deleting = $state<string | null>(null);

	async function load() {
		loading = true;
		try {
			clips = await api.clips();
		} finally {
			loading = false;
		}
	}

	async function remove(id: string) {
		deleting = id;
		try {
			await api.deleteClip(id);
			clips = clips.filter((c) => c.id !== id);
		} finally {
			deleting = null;
		}
	}

	function formatDuration(sec: number): string {
		const total = Math.max(0, Math.floor(sec));
		const m = Math.floor(total / 60);
		const s = total % 60;
		return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString('fr-FR');
	}

	function displayTitle(clip: Clip): string {
		const t = clip.title?.trim();
		return t ? t : 'Clip sans titre';
	}

	onMount(load);
</script>

<div class="mx-auto max-w-3xl px-6 py-10 md:px-10">
	<div class="mb-8 flex items-start justify-between gap-4">
		<div>
			<h1 class="text-2xl font-semibold tracking-tight text-foreground">Clips</h1>
			<p class="mt-1 text-sm text-muted-foreground">
				Les extraits enregistrés depuis la régie.
			</p>
		</div>
		<Button variant="outline" size="sm" onclick={load} disabled={loading}>
			<Icon icon="solar:refresh-linear" width={16} />
			Rafraîchir
		</Button>
	</div>

	{#if loading}
		<p class="text-sm text-muted-foreground">Chargement…</p>
	{:else if clips.length === 0}
		<div class="flex flex-col items-center gap-3 py-16 text-center">
			<Icon
				icon="solar:clapperboard-play-bold-duotone"
				width={48}
				class="text-muted-foreground"
			/>
			<p class="max-w-sm text-sm text-muted-foreground">
				Aucun clip pour l'instant. Enregistre-en un depuis la Prise d'antenne.
			</p>
		</div>
	{:else}
		<div class="space-y-4">
			{#each clips as clip (clip.id)}
				<Card>
					<div class="flex items-start justify-between gap-4">
						<div class="min-w-0">
							<h2 class="truncate text-base font-medium text-foreground">
								{displayTitle(clip)}
							</h2>
							<p class="mt-0.5 text-xs text-muted-foreground">
								durée {formatDuration(clip.durationSec)} · {formatDate(clip.createdAt)}
							</p>
						</div>
						<Button
							variant="ghost"
							size="icon"
							onclick={() => remove(clip.id)}
							disabled={deleting === clip.id}
							aria-label="Supprimer le clip"
						>
							<Icon icon="solar:trash-bin-trash-bold-duotone" width={20} />
						</Button>
					</div>

					<audio
						controls
						preload="none"
						src={clip.url}
						class="mt-4 w-full"
					></audio>

					<div class="mt-3">
						<a
							href={clip.url}
							download
							class="inline-flex h-8 items-center justify-center gap-2 rounded-[var(--radius)] bg-transparent px-3 text-sm font-medium text-foreground transition hover:bg-muted"
						>
							<Icon icon="solar:download-minimalistic-linear" width={16} />
							Télécharger
						</a>
					</div>
				</Card>
			{/each}
		</div>
	{/if}
</div>
