<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import Hls from 'hls.js';
	import Icon from '$lib/components/Icon.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import AudioVisualizer from '$lib/components/AudioVisualizer.svelte';
	import ChatWidget from '$lib/components/ChatWidget.svelte';
	import { api, type NowPlaying } from '$lib/api';

	let audio: HTMLAudioElement;
	let hls: Hls | null = null;
	let playing = $state(false);
	let needsGesture = $state(false);
	let np = $state<NowPlaying | null>(null);
	let audioCtx: AudioContext | null = null;
	let analyser = $state<AnalyserNode | null>(null);
	let graphReady = false;
	let poll: ReturnType<typeof setInterval>;

	let volume = $state(1);
	let muted = $state(false);
	let bg = $state('');

	const STREAM = '/stream/live.m3u8';

	function attach() {
		if (Hls.isSupported()) {
			hls = new Hls({ lowLatencyMode: true });
			hls.loadSource(STREAM);
			hls.attachMedia(audio);
		} else {
			audio.src = STREAM; // Safari native HLS
		}
	}

	function setupGraph() {
		if (graphReady) return;
		audioCtx = new AudioContext();
		const src = audioCtx.createMediaElementSource(audio);
		const an = audioCtx.createAnalyser();
		an.fftSize = 512;
		src.connect(an);
		an.connect(audioCtx.destination);
		analyser = an;
		graphReady = true;
	}

	async function start() {
		try {
			setupGraph();
			await audioCtx?.resume();
			audio.muted = muted;
			audio.volume = volume;
			await audio.play();
			playing = true;
			needsGesture = false;
		} catch {
			needsGesture = true;
			playing = false;
		}
	}

	function toggleMute() {
		muted = !muted;
		if (audio) audio.muted = muted;
	}

	$effect(() => {
		if (audio) audio.volume = volume;
	});

	async function refresh() {
		try {
			const pd = hls?.playingDate;
			np = await api.nowPlaying(pd ? pd.getTime() : undefined);
		} catch {
			/* keep last */
		}
	}

	onMount(() => {
		attach();
		start();
		refresh();
		poll = setInterval(refresh, 4000);
		api.appearance().then((a) => (bg = a.background)).catch(() => {});
	});

	onDestroy(() => {
		clearInterval(poll);
		hls?.destroy();
		audioCtx?.close();
	});
</script>

<audio bind:this={audio} class="hidden"></audio>

<main
	class="flex min-h-screen w-full flex-col items-center justify-center gap-8 px-6 py-12"
	style={bg ? `background: ${bg};` : ''}
>
	<div class="flex flex-col items-center gap-3">
		<Icon icon="solar:podcast-bold-duotone" width={64} />
		<h1 class="text-4xl font-bold tracking-tight">Antenne</h1>
	</div>

	<!-- Radial visualizer with a central control -->
	<div class="relative h-60 w-60">
		<AudioVisualizer {analyser} variant="radial" class="absolute inset-0 h-full w-full" />
		<div class="absolute inset-0 flex items-center justify-center">
			{#if needsGesture}
				<Button size="lg" class="h-20 w-20 rounded-full" onclick={start} aria-label="Activer le son">
					<Icon icon="solar:play-bold" width={32} />
				</Button>
			{:else}
				<button
					onclick={toggleMute}
					aria-label={muted ? 'Réactiver le son' : 'Couper le son'}
					class="flex h-20 w-20 items-center justify-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-foreground)] transition hover:opacity-90"
				>
					<Icon icon={muted ? 'solar:muted-bold-duotone' : 'solar:volume-loud-bold-duotone'} width={30} />
				</button>
			{/if}
		</div>
	</div>

	<!-- Volume -->
	<div class="flex w-full max-w-xs items-center gap-3">
		<button
			onclick={toggleMute}
			aria-label="Mute"
			class="text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]"
		>
			<Icon icon={muted || volume === 0 ? 'solar:muted-linear' : 'solar:volume-loud-linear'} width={20} />
		</button>
		<input
			type="range"
			min="0"
			max="1"
			step="0.01"
			bind:value={volume}
			aria-label="Volume"
			class="h-1.5 flex-1 cursor-pointer appearance-none rounded-full bg-[var(--color-muted)] accent-[var(--color-foreground)]"
		/>
	</div>

	{#if needsGesture}
		<p class="text-xs text-[var(--color-muted-foreground)]">Clique pour activer le son 🔊</p>
	{/if}

	<!-- Now playing -->
	<div class="flex flex-col items-center gap-1 text-center">
		{#if np?.live}
			<span
				class="flex items-center gap-2 rounded-full border border-red-500/40 px-3 py-1 text-xs font-medium text-red-500"
			>
				<span class="h-2 w-2 animate-pulse rounded-full bg-red-500"></span> EN DIRECT
			</span>
		{/if}
		<p class="mt-2 text-lg font-medium">{np?.title || 'Silence radio'}</p>
		{#if np?.artist}
			<p class="text-sm text-[var(--color-muted-foreground)]">{np.artist}</p>
		{/if}
	</div>

	<div class="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
		<Icon icon="solar:users-group-rounded-bold-duotone" width={18} />
		<span>{np?.listeners ?? 0} à l'écoute</span>
	</div>
</main>

<ChatWidget />
