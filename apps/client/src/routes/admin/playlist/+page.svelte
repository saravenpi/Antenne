<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, type Track, type NowPlaying, type Collection } from '$lib/api';
	import Icon from '$lib/components/Icon.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';

	let tracks = $state<Track[]>([]);
	let nowPlaying = $state<NowPlaying | null>(null);

	let collections = $state<Collection[]>([]);
	let selectedCollection = $state<string | null>(null);

	// Inline "new collection" input
	let creatingCollection = $state(false);
	let newCollectionName = $state('');

	// Inline rename of the selected collection
	let renaming = $state(false);
	let renameName = $state('');

	// Inline delete confirmation
	let confirmingDelete = $state(false);

	const activeCollection = $derived(collections.find((c) => c.active) ?? null);
	const selectedCollectionObj = $derived(
		selectedCollection === null ? null : (collections.find((c) => c.id === selectedCollection) ?? null)
	);
	const visibleTracks = $derived(
		selectedCollection === null
			? tracks
			: tracks.filter((t) => t.collectionId === selectedCollection)
	);

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

	async function loadCollections() {
		try {
			collections = await api.collections();
		} catch (e) {
			console.error(e);
		}
	}

	async function createCollection() {
		const name = newCollectionName.trim();
		if (!name) return;
		try {
			const created = await api.createCollection(name);
			newCollectionName = '';
			creatingCollection = false;
			await loadCollections();
			selectedCollection = created.id;
		} catch (e) {
			console.error(e);
		}
	}

	async function activate(id: string) {
		try {
			nowPlaying = await api.activateCollection(id);
			await loadCollections();
			await loadTracks();
		} catch (e) {
			console.error(e);
		}
	}

	async function clearActive() {
		try {
			nowPlaying = await api.clearActiveCollection();
			await loadCollections();
			await loadTracks();
		} catch (e) {
			console.error(e);
		}
	}

	function startRename() {
		renameName = selectedCollectionObj?.name ?? '';
		renaming = true;
	}

	async function confirmRename() {
		const name = renameName.trim();
		if (!selectedCollection || !name) return;
		try {
			await api.renameCollection(selectedCollection, name);
			renaming = false;
			await loadCollections();
		} catch (e) {
			console.error(e);
		}
	}

	async function deleteCollectionNow() {
		if (!selectedCollection) return;
		try {
			await api.deleteCollection(selectedCollection);
			confirmingDelete = false;
			selectedCollection = null;
			await loadCollections();
			await loadTracks();
		} catch (e) {
			console.error(e);
		}
	}

	function selectCollection(id: string | null) {
		selectedCollection = id;
		renaming = false;
		confirmingDelete = false;
	}

	async function moveTrackTo(id: string, value: string) {
		try {
			await api.moveTrack(id, value || null);
			await loadTracks();
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
				await api.upload(file, '', '', selectedCollection ?? undefined);
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
		const list = visibleTracks;
		const target = index + dir;
		if (target < 0 || target >= list.length) return;
		const idA = list[index].id;
		const idB = list[target].id;
		const ids = tracks.map((t) => t.id);
		const ia = ids.indexOf(idA);
		const ib = ids.indexOf(idB);
		if (ia === -1 || ib === -1) return;
		[ids[ia], ids[ib]] = [ids[ib], ids[ia]];
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
		loadCollections();
		pollTimer = setInterval(loadNowPlaying, 4000);
	});

	onDestroy(() => {
		if (pollTimer) clearInterval(pollTimer);
	});
</script>

<div class="mx-auto max-w-3xl px-4 py-6 sm:px-6 sm:py-10 md:px-10">
	<div class="mb-6 flex flex-wrap items-center justify-between gap-3">
		<div class="flex items-center gap-3">
			<h1 class="text-xl font-semibold text-[var(--color-foreground)] sm:text-2xl">Playlist</h1>
			<span
				class="rounded-full bg-[var(--color-muted)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-muted-foreground)]"
			>
				{visibleTracks.length} son{visibleTracks.length > 1 ? 's' : ''}
			</span>
		</div>
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

	<!-- Collections bar (tabs / chips) -->
	<div class="mb-4 flex gap-2 overflow-x-auto pb-1">
		<button
			type="button"
			class="flex shrink-0 items-center gap-1.5 rounded-full px-3 py-1.5 text-sm font-medium transition {selectedCollection ===
			null
				? 'bg-[var(--color-foreground)] text-[var(--color-background)]'
				: 'bg-[var(--color-muted)] text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]'}"
			onclick={() => selectCollection(null)}
		>
			<Icon icon="lucide:library" width={14} class="shrink-0" />
			Toutes
		</button>
		{#each collections as c (c.id)}
			<button
				type="button"
				class="flex shrink-0 items-center gap-1.5 rounded-full px-3 py-1.5 text-sm font-medium transition {selectedCollection ===
				c.id
					? 'bg-[var(--color-foreground)] text-[var(--color-background)]'
					: 'bg-[var(--color-muted)] text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]'}"
				onclick={() => selectCollection(c.id)}
			>
				<Icon icon="lucide:folder" width={14} class="shrink-0" />
				<span class="max-w-[10rem] truncate">{c.name}</span>
				{#if c.active}
					<span class="h-2 w-2 shrink-0 rounded-full bg-green-500" title="En diffusion"></span>
				{/if}
			</button>
		{/each}
		{#if creatingCollection}
			<div class="flex shrink-0 items-center gap-1">
				<Input
					bind:value={newCollectionName}
					placeholder="Nom de la playlist"
					class="h-8 w-40"
					autofocus
					onkeydown={(e: KeyboardEvent) => {
						if (e.key === 'Enter') createCollection();
						if (e.key === 'Escape') {
							creatingCollection = false;
							newCollectionName = '';
						}
					}}
				/>
				<Button variant="ghost" size="icon" class="h-8 w-8" aria-label="Créer" onclick={createCollection}>
					<Icon icon="lucide:check" width={16} />
				</Button>
			</div>
		{:else}
			<button
				type="button"
				class="flex shrink-0 items-center gap-1.5 rounded-full border border-dashed border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-muted-foreground)] transition hover:text-[var(--color-foreground)]"
				onclick={() => {
					creatingCollection = true;
					newCollectionName = '';
				}}
			>
				<Icon icon="lucide:folder-plus" width={14} class="shrink-0" />
				Nouvelle
			</button>
		{/if}
	</div>

	<!-- On-air control -->
	<Card class="mb-4 flex flex-wrap items-center justify-between gap-3 p-3">
		<div class="flex min-w-0 items-center gap-2 text-sm">
			<Icon icon="lucide:radio" width={16} class="shrink-0 text-green-500" />
			<span class="text-[var(--color-muted-foreground)]">En diffusion :</span>
			<span class="truncate font-medium text-[var(--color-foreground)]">
				{activeCollection ? activeCollection.name : 'Toute la bibliothèque'}
			</span>
		</div>
		<div class="flex shrink-0 flex-wrap gap-2">
			{#if selectedCollectionObj && !selectedCollectionObj.active}
				<Button variant="outline" onclick={() => activate(selectedCollectionObj!.id)}>
					<Icon icon="lucide:radio" width={16} />
					Diffuser cette playlist
				</Button>
			{/if}
			{#if activeCollection}
				<Button variant="outline" onclick={clearActive}>
					<Icon icon="lucide:library" width={16} />
					Diffuser toute la bibliothèque
				</Button>
			{/if}
		</div>
	</Card>

	<!-- Selected collection management -->
	{#if selectedCollectionObj}
		<div class="mb-6 flex flex-wrap items-center gap-2">
			{#if renaming}
				<Input
					bind:value={renameName}
					class="h-9 w-48"
					autofocus
					onkeydown={(e: KeyboardEvent) => {
						if (e.key === 'Enter') confirmRename();
						if (e.key === 'Escape') renaming = false;
					}}
				/>
				<Button variant="outline" onclick={confirmRename}>
					<Icon icon="lucide:check" width={16} />
					Enregistrer
				</Button>
				<Button variant="ghost" onclick={() => (renaming = false)}>
					<Icon icon="lucide:x" width={16} />
					Annuler
				</Button>
			{:else}
				<Button variant="outline" onclick={startRename}>
					<Icon icon="lucide:pencil" width={16} />
					Renommer
				</Button>
				{#if confirmingDelete}
					<Button
						variant="outline"
						class="text-red-500 hover:bg-red-500/10"
						onclick={deleteCollectionNow}
					>
						<Icon icon="lucide:trash-2" width={16} />
						Confirmer ?
					</Button>
					<Button variant="ghost" onclick={() => (confirmingDelete = false)}>
						<Icon icon="lucide:x" width={16} />
						Annuler
					</Button>
				{:else}
					<Button
						variant="outline"
						class="text-red-500 hover:bg-red-500/10"
						onclick={() => (confirmingDelete = true)}
					>
						<Icon icon="lucide:trash-2" width={16} />
						Supprimer
					</Button>
				{/if}
			{/if}
		</div>
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
	{#if visibleTracks.length === 0}
		<Card class="text-center text-sm text-[var(--color-muted-foreground)]">
			{selectedCollection === null
				? "Aucun son pour l'instant."
				: 'Aucun son dans cette playlist.'}
		</Card>
	{:else}
		<div class="flex flex-col gap-2">
			{#each visibleTracks as track, i (track.id)}
				{@const isNow = track.id === nowPlaying?.trackId}
				{@const isNext = track.id === nowPlaying?.next?.trackId}
				<Card
					class="flex flex-wrap items-center gap-3 p-3 {isNow
						? 'border-green-500/60 bg-[var(--color-muted)]'
						: ''}"
				>
					{#if track.coverUrl}
						<img
							src={track.coverUrl}
							alt=""
							class="h-10 w-10 shrink-0 rounded-md object-cover"
							loading="lazy"
						/>
					{:else}
						<span
							class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-[var(--color-muted)] text-[var(--color-muted-foreground)]"
							aria-hidden="true"
						>
							<Icon icon="lucide:music" width={18} />
						</span>
					{/if}
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
						<select
							class="h-8 max-w-[8rem] rounded-[var(--radius)] border border-[var(--color-border)] bg-transparent px-1.5 text-xs text-[var(--color-foreground)] outline-none focus:ring-2 focus:ring-[var(--color-foreground)]/20"
							title="Déplacer vers une playlist"
							aria-label="Déplacer {track.title} vers une playlist"
							value={track.collectionId ?? ''}
							onchange={(e) => moveTrackTo(track.id, (e.currentTarget as HTMLSelectElement).value)}
						>
							<option value="">Sans playlist</option>
							{#each collections as c (c.id)}
								<option value={c.id}>{c.name}</option>
							{/each}
						</select>
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
							disabled={i === visibleTracks.length - 1}
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
