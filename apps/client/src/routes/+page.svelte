<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import Hls from 'hls.js';
	import Icon from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import AudioVisualizer from '$lib/components/AudioVisualizer.svelte';
	import ChatWidget from '$lib/components/ChatWidget.svelte';
	import { api, type NowPlaying, type Social } from '$lib/api';
	import { socialIcon, socialLabel } from '$lib/socials';

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
	let stationName = $state('Antenne');
	let logo = $state('');
	let socials = $state<Social[]>([]);

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
		const Ctx =
			window.AudioContext ||
			(window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
		audioCtx = new Ctx();
		const src = audioCtx.createMediaElementSource(audio);
		const an = audioCtx.createAnalyser();
		an.fftSize = 512;
		src.connect(an);
		an.connect(audioCtx.destination);
		analyser = an;
		graphReady = true;
	}

	// Called from a user gesture (tap/click). Chrome's autoplay policy requires the
	// AudioContext to be created/resumed inside a gesture — otherwise it starts
	// "suspended" and, because the media element is routed through it, no sound
	// comes out. So the Web Audio graph is built here, not at mount.
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

	// Mount-time attempt: try to play WITHOUT touching Web Audio (no AudioContext
	// before a gesture). If the browser blocks unmuted autoplay we surface the
	// "tap to start" control; the graph + visualizer come up on that gesture.
	async function tryAutoplay() {
		try {
			audio.muted = false;
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
		tryAutoplay();
		refresh();
		poll = setInterval(refresh, 4000);
		api
			.appearance()
			.then((a) => {
				bg = a.background;
				if (a.stationName) stationName = a.stationName;
				logo = a.logo ?? '';
				socials = a.socials ?? [];
			})
			.catch(() => {});
	});

	onDestroy(() => {
		clearInterval(poll);
		hls?.destroy();
		audioCtx?.close();
	});
</script>

<audio bind:this={audio} class="hidden"></audio>

<!-- Station identity, top-left -->
<div class="fixed left-4 top-4 z-40 flex flex-col gap-1.5 sm:left-6 sm:top-6">
	<div class="flex max-w-[55vw] items-center gap-2.5 sm:max-w-none">
		<Logo src={logo} size={30} class="shrink-0" />
		<span class="truncate text-xl font-bold tracking-tight">{stationName}</span>
	</div>
	{#if socials.length}
		<div class="flex items-center gap-2 pl-0.5">
			{#each socials as s (s.platform + s.url)}
				<a
					href={s.url}
					target="_blank"
					rel="noopener noreferrer"
					aria-label={socialLabel(s.platform)}
					title={socialLabel(s.platform)}
					class="flex h-7 w-7 items-center justify-center rounded-full text-[var(--color-muted-foreground)] transition hover:text-[var(--color-foreground)]"
				>
					<Icon icon={socialIcon(s.platform)} width={18} />
				</a>
			{/each}
		</div>
	{/if}
</div>

<main
	class="flex min-h-screen w-full flex-col items-center justify-center gap-8 px-6 py-12"
	style={bg ? `background: ${bg};` : ''}
>
	<!-- Bars visualizer (same rendering as the régie mic) -->
	<AudioVisualizer {analyser} variant="bars" fallback={playing} class="h-40 w-full max-w-2xl" />

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
</main>

<!-- Listener count, floating liquid-glass pill: top-right on mobile, bottom-left from sm up -->
<div
	class="fixed right-4 top-4 z-40 flex h-12 items-center gap-2 rounded-full border border-border/40 bg-background/55 px-4 text-sm text-[var(--color-muted-foreground)] shadow-lg shadow-black/20 ring-1 ring-white/10 backdrop-blur-2xl backdrop-saturate-150 sm:right-auto sm:top-auto sm:left-6 sm:[bottom:max(1.5rem,env(safe-area-inset-bottom))]"
>
	<Icon icon="lucide:users" width={18} class="shrink-0" />
	<span class="relative flex h-2 w-2 shrink-0" aria-hidden="true">
		<span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-500 opacity-75"></span>
		<span class="relative inline-flex h-2 w-2 rounded-full bg-green-500"></span>
	</span>
	<span class="whitespace-nowrap">{np?.listeners ?? 0} à l'écoute</span>
</div>

<!-- Sound controls, floating liquid glass at bottom-center -->
<div
	class="fixed left-1/2 z-40 flex -translate-x-1/2 items-center gap-3 [bottom:max(1rem,env(safe-area-inset-bottom))] sm:[bottom:max(1.5rem,env(safe-area-inset-bottom))]"
>
	<div
		class="flex h-12 items-center gap-3 rounded-full border border-border/40 bg-background/55 px-4 shadow-lg shadow-black/20 ring-1 ring-white/10 backdrop-blur-2xl backdrop-saturate-150"
	>
		<Icon
			icon={muted || volume === 0 ? 'lucide:volume-off' : 'lucide:volume-2'}
			width={18}
			class="shrink-0 text-[var(--color-muted-foreground)]"
		/>
		<input
			type="range"
			min="0"
			max="1"
			step="0.01"
			bind:value={volume}
			aria-label="Volume"
			class="h-1.5 w-20 cursor-pointer appearance-none rounded-full bg-[var(--color-muted)] accent-[var(--color-foreground)] sm:w-40"
		/>
	</div>
	<button
		onclick={() => (needsGesture ? start() : toggleMute())}
		aria-label={needsGesture ? 'Activer le son' : muted ? 'Réactiver le son' : 'Couper le son'}
		class="flex h-12 w-12 shrink-0 items-center justify-center rounded-full border border-border/40 bg-background/55 text-[var(--color-foreground)] shadow-lg shadow-black/20 ring-1 ring-white/10 backdrop-blur-2xl backdrop-saturate-150 transition hover:bg-background/70"
	>
		<Icon
			icon={needsGesture
				? 'lucide:play'
				: muted
					? 'lucide:volume-off'
					: 'lucide:volume-2'}
			width={22}
		/>
	</button>
</div>

<ChatWidget />
