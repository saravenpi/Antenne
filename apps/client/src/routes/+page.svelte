<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import Hls from 'hls.js';
	import AudioVisualizer from '$lib/components/AudioVisualizer.svelte';
	import ChatWidget from '$lib/components/ChatWidget.svelte';
	import StationHeader from '$lib/components/player/StationHeader.svelte';
	import ListenerBadge from '$lib/components/player/ListenerBadge.svelte';
	import NowPlayingBlock from '$lib/components/player/NowPlaying.svelte';
	import SoundControls from '$lib/components/player/SoundControls.svelte';
	import { api, type NowPlaying, type Social } from '$lib/api';

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
			// The backend is standard (not low-latency) HLS, so DON'T ride the live
			// edge: keep a few segments of buffer. Low-latency mode left almost no
			// cushion, so any server pacing hiccup (e.g. the brief stall while ffmpeg
			// spawns the next track's decoder) made playback stall and cut out.
			hls = new Hls({
				lowLatencyMode: false,
				liveSyncDurationCount: 4,
				liveMaxLatencyDurationCount: 12,
				maxBufferLength: 30,
				backBufferLength: 30
			});
			hls.loadSource(STREAM);
			hls.attachMedia(audio);
			// Recover from transient network/media errors instead of dying.
			hls.on(Hls.Events.ERROR, (_e, data) => {
				if (!data.fatal) return;
				if (data.type === Hls.ErrorTypes.NETWORK_ERROR) hls?.startLoad();
				else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) hls?.recoverMediaError();
				else {
					hls?.destroy();
					attach();
					if (playing) audio.play().catch(() => {});
				}
			});
		} else {
			audio.src = STREAM; // Safari native HLS
		}
	}

	// If playback stalls (or the tab was backgrounded and fell far behind), nudge
	// back toward the live edge and resume.
	function nudgeLive() {
		if (!playing) return;
		try {
			const s = audio.seekable;
			if (s.length) {
				const edge = s.end(s.length - 1);
				if (edge - audio.currentTime > 12) audio.currentTime = edge - 3;
			}
		} catch {
			/* ignore */
		}
		audio.play().catch(() => {});
	}

	function newCtx(): AudioContext {
		const Ctx =
			window.AudioContext ||
			(window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
		return new Ctx();
	}

	// Route the (already-playing) media element through an analyser so the
	// visualizer reacts to the real audio. Only valid on a RUNNING context.
	function connectGraph(ctx: AudioContext) {
		const src = ctx.createMediaElementSource(audio);
		const an = ctx.createAnalyser();
		an.fftSize = 512;
		src.connect(an);
		an.connect(ctx.destination);
		audioCtx = ctx;
		analyser = an;
		graphReady = true;
	}

	function setupGraph() {
		if (graphReady) return;
		connectGraph(newCtx());
	}

	// Try to wire the analyser WITHOUT a user gesture. On high-media-engagement
	// origins Chrome lets the AudioContext start/resume running with no gesture,
	// so the visualizer reacts immediately instead of only after a click. If it
	// stays suspended we bail (never calling createMediaElementSource, which
	// would mute the element) and leave it to the first interaction.
	async function tryGraphNoGesture() {
		if (graphReady) return;
		let ctx: AudioContext;
		try {
			ctx = newCtx();
		} catch {
			return;
		}
		if (ctx.state === 'suspended') await ctx.resume().catch(() => {});
		if (ctx.state === 'running') connectGraph(ctx);
		else ctx.close().catch(() => {});
	}

	// Build + resume the Web Audio graph. Safe to call repeatedly; must be invoked
	// from within a user gesture (autoplay policy).
	function ensureGraph() {
		if (graphReady) return;
		try {
			setupGraph();
		} catch {
			return;
		}
		audioCtx?.resume().catch(() => {});
	}

	// Autoplay can succeed silently (high media-engagement) without the play
	// button ever being pressed — so the graph is never built and the visualizer
	// has no analyser data. Build it on the first user interaction anywhere.
	const GESTURES = ['pointerdown', 'keydown', 'touchstart'] as const;
	function primeGraph() {
		ensureGraph();
		if (graphReady && typeof window !== 'undefined')
			for (const ev of GESTURES) window.removeEventListener(ev, primeGraph, true);
	}

	// Called from a user gesture (tap/click). Chrome's autoplay policy requires the
	// AudioContext to be created/resumed inside a gesture — otherwise it starts
	// "suspended" and, because the media element is routed through it, no sound
	// comes out. So the Web Audio graph is built here, not at mount.
	async function start() {
		try {
			ensureGraph();
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
		(async () => {
			await tryAutoplay();
			// If autoplay worked (high engagement), try to wire the analyser now so
			// the visualizer reacts without needing a click.
			if (playing) await tryGraphNoGesture();
		})();
		for (const ev of GESTURES) window.addEventListener(ev, primeGraph, true);
		audio.addEventListener('stalled', nudgeLive);
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
		if (typeof window !== 'undefined')
			for (const ev of GESTURES) window.removeEventListener(ev, primeGraph, true);
		audio?.removeEventListener('stalled', nudgeLive);
		hls?.destroy();
		audioCtx?.close();
	});
</script>

<audio bind:this={audio} class="hidden"></audio>

<StationHeader {logo} {stationName} {socials} />

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
	<NowPlayingBlock {np} />
</main>

<!-- Listener count, floating liquid-glass pill: top-right on mobile, bottom-left from sm up -->
<ListenerBadge count={np?.listeners ?? 0} />

<!-- Sound controls, floating liquid glass at bottom-center -->
<SoundControls
	bind:volume
	{muted}
	{needsGesture}
	onprimary={() => (needsGesture ? start() : toggleMute())}
/>

<ChatWidget />
