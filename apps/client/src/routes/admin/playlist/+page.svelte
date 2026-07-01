<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, type Track, type NowPlaying } from '$lib/api';
	import Icon from '$lib/components/Icon.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Button from '$lib/components/ui/Button.svelte';

	let tracks = $state<Track[]>([]);
	let nowPlaying = $state<NowPlaying | null>(null);

	let dragOver = $state(false);
	let fileInput: HTMLInputElement;
	let queue = $state<{ name: string; status: 'pending' | 'uploading' | 'done' | 'error' }[]>([]);
	let uploading = $state(false);

	let cleaning = $state(false);
	let cleanError = $state('');

	let controlBusy = $state(false);

	let pollTimer: ReturnType<typeof setInterval> | undefined;

	async function loadTracks() {
		try {
			tracks = await api.tracks();
		} catch (e) {
			console.error(e);
		}
	}

	async function loadNowPlaying() {
		try {
			nowPlaying = await api.nowPlaying();
		} catch (e) {
			console.error(e);
		}
	}

	function isAudio(file: File): boolean {
		return file.type.startsWith('audio/') || /\.(mp3|wav|flac|ogg|m4a|aac)$/i.test(file.name);
	}

	async function handleFiles(files: File[]) {
		const audio = files.filter(isAudio);
		if (audio.length === 0) return;

		queue = audio.map((f) => ({ name: f.name, status: 'pending' as const }));
		uploading = true;

		for (let i = 0; i < audio.length; i++) {
			const file = audio[i];
			queue[i] = { ...queue[i], status: 'uploading' };
			try {
				await api.upload(file, '', '');
				queue[i] = { ...queue[i], status: 'done' };
				await loadTracks();
			} catch (e) {
				console.error(e);
				queue[i] = { ...queue[i], status: 'error' };
			}
		}

		uploading = false;
	}

	function onDrop(e: DragEvent) {
		e.preventDefault();
		dragOver = false;
		if (!e.dataTransfer) return;
		handleFiles(Array.from(e.dataTransfer.files));
	}

	function onSelect(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		if (input.files) handleFiles(Array.from(input.files));
		input.value = '';
	}

	async function cleanMetadata() {
		cleaning = true;
		cleanError = '';
		try {
			tracks = await api.rescanMetadata();
		} catch (e) {
			cleanError = e instanceof Error ? e.message : String(e);
		} finally {
			cleaning = false;
		}
	}

	async function runControl(action: () => Promise<NowPlaying>) {
		if (controlBusy) return;
		controlBusy = true;
		try {
			nowPlaying = await action();
		} catch (e) {
			console.error(e);
		} finally {
			controlBusy = false;
		}
	}

	async function playNow(id: string) {
		if (controlBusy) return;
		controlBusy = true;
		try {
			nowPlaying = await api.playTrack(id);
		} catch (e) {
			console.error(e);
		} finally {
			controlBusy = false;
		}
	}

	async function move(index: number, dir: -1 | 1) {
		const target = index + dir;
		if (target < 0 || target >= tracks.length) return;
		const ids = tracks.map((t) => t.id);
		[ids[index], ids[target]] = [ids[target], ids[index]];
		try {
			await api.reorder(ids);
			await loadTracks();
		} catch (e) {
			console.error(e);
		}
	}

	async function remove(id: string) {
		try {
			await api.deleteTrack(id);
			await loadTracks();
		} catch (e) {
			console.error(e);
		}
	}

	onMount(() => {
		loadTracks();
		loadNowPlaying();
		pollTimer = setInterval(loadNowPlaying, 4000);
	});

	onDestroy(() => {
		if (pollTimer) clearInterval(pollTimer);
	});
</script>

<div class="mx-auto max-w-3xl px-4 py-6 sm:px-6 sm:py-10 md:px-10">
	<div class="mb-6 flex flex-wrap items-center justify-between gap-3">
		<h1 class="text-xl font-semibold text-[var(--color-foreground)] sm:text-2xl">
			Playlist ({tracks.length})
		</h1>
		<Button variant="outline" disabled={cleaning} onclick={cleanMetadata}>
			{#if cleaning}
				<Icon icon="lucide:loader-circle" width={18} class="animate-spin" />
				Nettoyage…
			{:else}
				<Icon icon="lucide:wand-sparkles" width={18} />
				Nettoyer les métadonnées
			{/if}
		</Button>
	</div>
	{#if cleanError}
		<p class="mb-6 text-sm text-red-500">{cleanError}</p>
	{/if}

	<!-- Transport control bar -->
	<Card class="mb-6 flex flex-wrap items-center gap-3 p-3">
		<div class="flex items-center gap-1">
			<Button
				variant="ghost"
				size="icon"
				class="h-9 w-9"
				aria-label="Précédent"
				title="Précédent"
				disabled={controlBusy}
				onclick={() => runControl(api.playPrevious)}
			>
				<Icon icon="lucide:skip-back" width={20} />
			</Button>
			{#if nowPlaying?.paused}
				<Button
					variant="ghost"
					size="icon"
					class="h-9 w-9 text-green-500"
					aria-label="Reprendre"
					title="Reprendre"
					disabled={controlBusy}
					onclick={() => runControl(api.resumePlayback)}
				>
					<Icon icon="lucide:play" width={22} />
				</Button>
			{:else}
				<Button
					variant="ghost"
					size="icon"
					class="h-9 w-9"
					aria-label="Pause"
					title="Pause"
					disabled={controlBusy}
					onclick={() => runControl(api.pausePlayback)}
				>
					<Icon icon="lucide:pause" width={22} />
				</Button>
			{/if}
			<Button
				variant="ghost"
				size="icon"
				class="h-9 w-9"
				aria-label="Suivant"
				title="Suivant"
				disabled={controlBusy}
				onclick={() => runControl(api.playNext)}
			>
				<Icon icon="lucide:skip-forward" width={20} />
			</Button>
		</div>
		<div class="min-w-0 flex-1">
			{#if nowPlaying?.paused}
				<div class="flex items-center gap-1.5 text-sm font-medium text-[var(--color-muted-foreground)]">
					<Icon icon="lucide:pause" width={14} class="shrink-0" />
					<span class="truncate">En pause — {nowPlaying?.title ?? 'silence'}</span>
				</div>
			{:else if nowPlaying?.title}
				<div class="flex items-center gap-1.5 text-sm font-medium text-[var(--color-foreground)]">
					<Icon icon="lucide:play" width={14} class="shrink-0 text-green-500" />
					<span class="truncate">{nowPlaying.title}</span>
				</div>
			{:else}
				<span class="text-sm text-[var(--color-muted-foreground)]">Aucune lecture</span>
			{/if}
		</div>
	</Card>

	<!-- Multi-file drop zone -->
	<div
		role="button"
		tabindex="0"
		class="mb-8 flex cursor-pointer flex-col items-center justify-center gap-2 rounded-[var(--radius)] border-2 border-dashed px-4 py-10 text-center transition sm:px-6 sm:py-12 {dragOver
			? 'border-[var(--color-foreground)] bg-[var(--color-muted)]'
			: 'border-[var(--color-border)]'}"
		ondragover={(e) => {
			e.preventDefault();
			dragOver = true;
		}}
		ondragleave={() => (dragOver = false)}
		ondrop={onDrop}
		onclick={() => fileInput.click()}
		onkeydown={(e) => {
			if (e.key === 'Enter' || e.key === ' ') fileInput.click();
		}}
	>
		<Icon
			icon="lucide:cloud-upload"
			width={48}
			class="text-[var(--color-muted-foreground)]"
		/>
		<p class="text-sm font-medium text-[var(--color-foreground)]">
			{uploading ? 'Envoi…' : 'Glisse des fichiers audio ici (plusieurs possibles)'}
		</p>
		<p class="text-xs text-[var(--color-muted-foreground)]">
			ou clique pour parcourir · MP3, WAV, FLAC…
		</p>
		<input
			bind:this={fileInput}
			type="file"
			accept="audio/*"
			multiple
			class="hidden"
			onchange={onSelect}
		/>
	</div>

	<!-- Upload queue -->
	{#if queue.length > 0}
		<div class="mb-8 flex flex-col gap-1">
			{#each queue as item (item.name)}
				<div class="flex items-center gap-2 text-sm">
					{#if item.status === 'uploading'}
						<Icon
							icon="lucide:loader-circle"
							width={16}
							class="shrink-0 animate-spin text-[var(--color-muted-foreground)]"
						/>
					{:else if item.status === 'done'}
						<Icon icon="lucide:circle-check-big" width={16} class="shrink-0 text-green-500" />
					{:else if item.status === 'error'}
						<Icon icon="lucide:circle-x" width={16} class="shrink-0 text-red-500" />
					{:else}
						<Icon
							icon="lucide:clock"
							width={16}
							class="shrink-0 text-[var(--color-muted-foreground)]"
						/>
					{/if}
					<span class="truncate text-[var(--color-foreground)]">{item.name}</span>
				</div>
			{/each}
		</div>
	{/if}

	<!-- Ordered track list -->
	{#if tracks.length === 0}
		<Card class="text-center text-sm text-[var(--color-muted-foreground)]">
			Aucun son pour l'instant.
		</Card>
	{:else}
		<div class="flex flex-col gap-2">
			{#each tracks as track, i (track.id)}
				{@const isNow = track.id === nowPlaying?.trackId}
				{@const isNext = track.id === nowPlaying?.next?.trackId}
				<Card
					class="flex items-center gap-3 p-3 {isNow
						? 'border-green-500/60 bg-[var(--color-muted)]'
						: ''}"
				>
					<Button
						variant="ghost"
						size="icon"
						class="h-9 w-9 shrink-0"
						aria-label="Lire {track.title}"
						title="Lire maintenant"
						disabled={controlBusy}
						onclick={() => playNow(track.id)}
					>
						<Icon
							icon="lucide:play"
							width={18}
							class={isNow ? 'text-green-500' : ''}
						/>
					</Button>
					{#if isNow}
						<span
							class="eq flex h-4 w-6 shrink-0 items-end justify-center gap-0.5 {nowPlaying?.paused
								? 'eq-paused'
								: ''}"
							aria-hidden="true"
						>
							<span class="eq-bar"></span>
							<span class="eq-bar"></span>
							<span class="eq-bar"></span>
							<span class="eq-bar"></span>
						</span>
					{:else}
						<span
							class="w-6 shrink-0 text-center text-sm tabular-nums text-[var(--color-muted-foreground)]"
						>
							{i + 1}
						</span>
					{/if}
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-2">
							<span class="truncate font-medium text-[var(--color-foreground)]">
								{track.title}
							</span>
							{#if isNow}
								<span
									class="shrink-0 rounded-[var(--radius)] px-1.5 py-0.5 text-[10px] font-semibold tracking-wide text-green-500"
								>
									● EN LECTURE
								</span>
							{:else if isNext}
								<span
									class="shrink-0 rounded-[var(--radius)] px-1.5 py-0.5 text-[10px] font-semibold tracking-wide text-[var(--color-muted-foreground)]"
								>
									À SUIVRE
								</span>
							{/if}
						</div>
						{#if track.artist}
							<div class="truncate text-xs text-[var(--color-muted-foreground)]">
								{track.artist}
							</div>
						{/if}
					</div>
					<div class="flex shrink-0 items-center gap-1">
						<Button
							variant="ghost"
							size="icon"
							class="h-8 w-8"
							disabled={i === 0}
							onclick={() => move(i, -1)}
						>
							<Icon icon="lucide:chevron-up" width={18} />
						</Button>
						<Button
							variant="ghost"
							size="icon"
							class="h-8 w-8"
							disabled={i === tracks.length - 1}
							onclick={() => move(i, 1)}
						>
							<Icon icon="lucide:chevron-down" width={18} />
						</Button>
						<Button
							variant="ghost"
							size="icon"
							class="h-8 w-8 text-red-500 hover:bg-red-500/10"
							onclick={() => remove(track.id)}
						>
							<Icon icon="lucide:trash-2" width={18} />
						</Button>
					</div>
				</Card>
			{/each}
		</div>
	{/if}
</div>

<style>
	.eq-bar {
		width: 3px;
		height: 30%;
		border-radius: 1px;
		background-color: var(--color-green-500, #22c55e);
		transform-origin: bottom;
		animation: eq-bounce 0.9s ease-in-out infinite;
	}
	.eq-bar:nth-child(1) {
		animation-delay: -0.2s;
	}
	.eq-bar:nth-child(2) {
		animation-delay: -0.5s;
	}
	.eq-bar:nth-child(3) {
		animation-delay: -0.1s;
	}
	.eq-bar:nth-child(4) {
		animation-delay: -0.7s;
	}
	.eq-paused .eq-bar {
		animation-play-state: paused;
		height: 35%;
	}

	@keyframes eq-bounce {
		0%,
		100% {
			transform: scaleY(0.35);
		}
		50% {
			transform: scaleY(1);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.eq-bar {
			animation: none;
			height: 40%;
		}
	}
</style>
