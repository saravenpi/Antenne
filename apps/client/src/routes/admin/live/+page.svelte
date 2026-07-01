<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { scale } from 'svelte/transition';
	import Icon from '$lib/components/Icon.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import AudioVisualizer from '$lib/components/AudioVisualizer.svelte';
	import { api, getToken, type NowPlaying } from '$lib/api';

	// ---- Live mic ----
	let live = $state(false);
	let starting = $state(false);
	let stopping = $state(false);
	let error = $state('');
	let ws: WebSocket | null = null;
	let recorder: MediaRecorder | null = null;
	let micStream: MediaStream | null = null;
	let audioCtx: AudioContext | null = null;
	let analyser = $state<AnalyserNode | null>(null);

	// ---- Mic input selection ----
	let devices = $state<MediaDeviceInfo[]>([]);
	let selectedDeviceId = $state('');
	let micLabelsKnown = $state(false);

	async function refreshDevices() {
		if (typeof navigator === 'undefined' || !navigator.mediaDevices?.enumerateDevices) return;
		try {
			const all = await navigator.mediaDevices.enumerateDevices();
			devices = all.filter((d) => d.kind === 'audioinput');
			micLabelsKnown = devices.some((d) => d.label !== '');
			// Drop a stale selection (device was unplugged).
			if (selectedDeviceId && !devices.some((d) => d.deviceId === selectedDeviceId)) {
				selectedDeviceId = '';
			}
		} catch {
			/* ignore — enumeration is best-effort */
		}
	}

	// Browsers hide device labels/ids until mic access is granted at least once.
	// Probe for permission, then re-enumerate so the real input names show up.
	async function requestMicAccess() {
		error = '';
		try {
			const probe = await navigator.mediaDevices.getUserMedia({ audio: true });
			probe.getTracks().forEach((t) => t.stop());
		} catch (e) {
			error = (e as Error).message;
		}
		await refreshDevices();
	}

	async function startLive() {
		if (live || starting) return;
		starting = true;
		error = '';
		try {
			micStream = await navigator.mediaDevices.getUserMedia({
				audio: selectedDeviceId ? { deviceId: { exact: selectedDeviceId } } : true
			});
			refreshDevices();
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
				recorder.ondataavailable = (ev) => {
					if (ev.data.size > 0 && ws?.readyState === WebSocket.OPEN) ws.send(ev.data);
				};
				recorder.start(250);
				live = true;
				starting = false;
			};
			ws.onclose = () => teardownLive();
		} catch (e) {
			error = (e as Error).message;
			starting = false;
			teardownLive();
		}
	}

	// Hard, synchronous teardown of everything. Idempotent — safe to call twice
	// (e.g. once from stopLive and again from ws.onclose).
	function teardownLive() {
		if (recorder && recorder.state !== 'inactive') recorder.stop();
		micStream?.getTracks().forEach((t) => t.stop());
		if (ws && ws.readyState !== WebSocket.CLOSED) ws.close();
		if (audioCtx && audioCtx.state !== 'closed') audioCtx.close();
		recorder = null;
		micStream = null;
		ws = null;
		audioCtx = null;
		analyser = null;
		live = false;
	}

	// Stop the mic/audio pipeline but keep the audioCtx/mic alive until AFTER the
	// recorder has flushed its final chunk over the socket.
	function stopMedia() {
		micStream?.getTracks().forEach((t) => t.stop());
		if (audioCtx && audioCtx.state !== 'closed') audioCtx.close();
		micStream = null;
		audioCtx = null;
		analyser = null;
		recorder = null;
		ws = null;
		live = false;
	}

	async function stopLive() {
		if (!live || stopping) return;
		stopping = true;

		// Wait for the recorder to flush its final chunk, then close the socket so
		// that chunk is transmitted before the close frame. If there's no active
		// recorder, tear down directly.
		await new Promise<void>((resolve) => {
			const activeWs = ws;
			if (recorder && recorder.state !== 'inactive') {
				recorder.onstop = () => {
					if (activeWs && activeWs.readyState !== WebSocket.CLOSED) activeWs.close();
					stopMedia();
					resolve();
				};
				recorder.stop();
			} else {
				teardownLive();
				resolve();
			}
		});

		await api.stopLive().catch(() => {});
		stopping = false;
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
		refreshDevices();
		navigator.mediaDevices?.addEventListener?.('devicechange', refreshDevices);
	});

	onDestroy(() => {
		teardownLive();
		clearInterval(nowTimer);
		clearTimeout(clipTimer);
		navigator.mediaDevices?.removeEventListener?.('devicechange', refreshDevices);
	});
</script>

<div class="mx-auto max-w-3xl px-4 py-6 sm:px-6 sm:py-10 md:px-10">
	<h1 class="text-xl font-semibold tracking-tight text-foreground sm:text-2xl">Prise d'antenne</h1>

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

		{#if analyser}
			<div class="w-full" transition:scale={{ duration: 300, start: 0.9, opacity: 0 }}>
				<AudioVisualizer {analyser} variant="bars" class="h-28 w-full" />
			</div>
		{/if}

		{#if !live}
			<div class="w-full max-w-sm">
				<label for="mic-input" class="mb-1.5 block text-sm font-medium text-foreground">
					Entrée micro
				</label>
				<div class="flex items-center gap-2">
					<select
						id="mic-input"
						bind:value={selectedDeviceId}
						disabled={starting}
						class="min-h-11 w-full rounded-[var(--radius)] border border-border bg-transparent px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-[var(--color-foreground)]/20 disabled:opacity-50"
					>
						<option value="">Micro par défaut</option>
						{#each devices as d, i (d.deviceId)}
							<option value={d.deviceId}>{d.label || `Microphone ${i + 1}`}</option>
						{/each}
					</select>
					<Button
						variant="outline"
						size="icon"
						class="min-h-11 shrink-0"
						title="Rafraîchir la liste des micros"
						aria-label="Rafraîchir les micros"
						disabled={starting}
						onclick={requestMicAccess}
					>
						<Icon icon="lucide:refresh-cw" width={18} />
					</Button>
				</div>
				{#if !micLabelsKnown}
					<button
						type="button"
						class="mt-1.5 text-xs text-muted-foreground underline-offset-2 hover:underline"
						onclick={requestMicAccess}
					>
						Autoriser l'accès pour voir les micros disponibles
					</button>
				{/if}
			</div>
		{/if}

		{#if live}
			<Button variant="destructive" size="lg" class="min-h-11 w-full sm:w-auto" disabled={stopping} onclick={stopLive}>
				{#if stopping}
					<Icon icon="lucide:loader-circle" width={20} class="animate-spin" />
					Coupure…
				{:else}
					<Icon icon="lucide:square" width={20} />
					Rendre l'antenne
				{/if}
			</Button>
		{:else}
			<Button size="lg" class="min-h-11 w-full sm:w-auto" disabled={starting || stopping} onclick={startLive}>
				{#if starting}
					<Icon icon="lucide:loader-circle" width={20} class="animate-spin" />
					Connexion…
				{:else}
					<Icon icon="lucide:mic" width={20} />
					Prendre le micro
				{/if}
			</Button>
		{/if}

		{#if error}
			<p class="text-sm text-red-500">{error}</p>
		{/if}
	</Card>

	<!-- Clipper card -->
	<Card class="mt-6">
		<div class="flex items-start gap-3">
			<Icon icon="lucide:clapperboard" width={24} class="mt-0.5 shrink-0 text-muted-foreground" />
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
