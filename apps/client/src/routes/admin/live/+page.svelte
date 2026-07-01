<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import AudioVisualizer from '$lib/components/AudioVisualizer.svelte';
	import { api, getToken, type NowPlaying } from '$lib/api';

	// ---- Live mic ----
	let live = $state(false);
	let error = $state('');
	let ws: WebSocket | null = null;
	let recorder: MediaRecorder | null = null;
	let micStream: MediaStream | null = null;
	let audioCtx: AudioContext | null = null;
	let analyser = $state<AnalyserNode | null>(null);

	async function startLive() {
		error = '';
		try {
			micStream = await navigator.mediaDevices.getUserMedia({ audio: true });
			audioCtx = new AudioContext();
			const src = audioCtx.createMediaStreamSource(micStream);
			const an = audioCtx.createAnalyser();
			an.fftSize = 256;
			src.connect(an);
			analyser = an;
			const proto = location.protocol === 'https:' ? 'wss' : 'ws';
			ws = new WebSocket(`${proto}://${location.host}/api/live/ingest?token=${getToken()}`);
			ws.binaryType = 'arraybuffer';
			ws.onopen = () => {
				recorder = new MediaRecorder(micStream!, { mimeType: 'audio/webm;codecs=opus' });
				recorder.ondataavailable = async (ev) => {
					if (ev.data.size > 0 && ws?.readyState === WebSocket.OPEN) ws.send(await ev.data.arrayBuffer());
				};
				recorder.start(250);
				live = true;
			};
			ws.onclose = () => teardownLive();
		} catch (e) {
			error = (e as Error).message;
			teardownLive();
		}
	}

	function teardownLive() {
		if (recorder && recorder.state !== 'inactive') recorder.stop();
		micStream?.getTracks().forEach((t) => t.stop());
		ws?.close();
		audioCtx?.close();
		recorder = null;
		micStream = null;
		ws = null;
		audioCtx = null;
		analyser = null;
		live = false;
	}

	async function stopLive() {
		teardownLive();
		await api.stopLive().catch(() => {});
	}

	// ---- Clips ----
	let clipMsg = $state('');
	let clipError = $state('');
	let clipping = $state(false);
	let clipTimer: ReturnType<typeof setTimeout> | undefined;

	async function clip(seconds: number) {
		clipError = '';
		clipping = true;
		try {
			await api.createClip(seconds);
			clipMsg = 'Clip enregistré ✓';
			clearTimeout(clipTimer);
			clipTimer = setTimeout(() => (clipMsg = ''), 3000);
		} catch (e) {
			clipError = (e as Error).message;
		} finally {
			clipping = false;
		}
	}

	// ---- Now playing ----
	let now = $state<NowPlaying | null>(null);
	let nowTimer: ReturnType<typeof setInterval> | undefined;

	async function refreshNow() {
		now = await api.nowPlaying().catch(() => now);
	}

	onMount(() => {
		refreshNow();
		nowTimer = setInterval(refreshNow, 5000);
	});

	onDestroy(() => {
		teardownLive();
		clearInterval(nowTimer);
		clearTimeout(clipTimer);
	});
</script>

<div class="mx-auto max-w-3xl px-6 py-10 md:px-10">
	<h1 class="text-2xl font-semibold tracking-tight text-foreground">Prise d'antenne</h1>

	<!-- Main live card -->
	<Card class="mt-6 flex flex-col items-center gap-6">
		{#if live}
			<div
				class="inline-flex items-center gap-2 rounded-full border border-red-600/40 bg-red-600/10 px-3 py-1 text-sm font-medium text-red-500"
			>
				<span class="relative flex h-2.5 w-2.5">
					<span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-500 opacity-75"></span>
					<span class="relative inline-flex h-2.5 w-2.5 rounded-full bg-red-500"></span>
				</span>
				EN DIRECT
			</div>
		{/if}

		<AudioVisualizer {analyser} variant="bars" class="h-28 w-full" />

		{#if live}
			<Button variant="destructive" size="lg" onclick={stopLive}>
				<Icon icon="solar:stop-bold" width={20} />
				Rendre l'antenne
			</Button>
		{:else}
			<Button size="lg" onclick={startLive}>
				<Icon icon="solar:microphone-3-bold-duotone" width={20} />
				Prendre le micro
			</Button>
		{/if}

		{#if error}
			<p class="text-sm text-red-500">{error}</p>
		{/if}
	</Card>

	<!-- Clipper card -->
	<Card class="mt-6">
		<div class="flex items-start gap-3">
			<Icon icon="solar:clapperboard-play-bold-duotone" width={24} class="mt-0.5 text-muted-foreground" />
			<div>
				<h2 class="text-lg font-medium text-foreground">Clipper</h2>
				<p class="text-sm text-muted-foreground">Enregistre les dernières secondes de l'antenne</p>
			</div>
		</div>

		<div class="mt-4 flex flex-wrap gap-3">
			<Button variant="outline" disabled={clipping} onclick={() => clip(15)}>15 s</Button>
			<Button variant="outline" disabled={clipping} onclick={() => clip(30)}>30 s</Button>
			<Button variant="outline" disabled={clipping} onclick={() => clip(60)}>60 s</Button>
		</div>

		{#if clipMsg}
			<p class="mt-3 text-sm text-foreground">{clipMsg}</p>
		{/if}
		{#if clipError}
			<p class="mt-3 text-sm text-red-500">{clipError}</p>
		{/if}
	</Card>

	<!-- Now playing readout -->
	{#if now}
		<div class="mt-6 space-y-1 text-sm text-muted-foreground">
			<p>En ce moment : <span class="text-foreground">{now.title}</span></p>
			{#if now.next}
				<p>À suivre : <span class="text-foreground">{now.next.title}</span></p>
			{/if}
		</div>
	{/if}
</div>
