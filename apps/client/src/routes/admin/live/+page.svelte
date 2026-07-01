<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import { scale } from 'svelte/transition';
	import Icon from '$lib/components/Icon.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import AudioVisualizer from '$lib/components/AudioVisualizer.svelte';
	import {
		api,
		getToken,
		chatWsUrl,
		type NowPlaying,
		type ChatMessage,
		type ChatEvent
	} from '$lib/api';

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

	// Safari exposes AudioContext under a webkit prefix on older iOS/macOS.
	function newAudioContext(): AudioContext {
		const Ctx =
			window.AudioContext ||
			(window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
		return new Ctx();
	}

	// Safari's MediaRecorder can't produce audio/webm — it only does audio/mp4
	// (AAC). Pick the first container the current browser actually supports; the
	// server decodes whatever arrives via ffmpeg, so any of these works.
	function pickRecorderMime(): string | undefined {
		const types = [
			'audio/webm;codecs=opus',
			'audio/webm',
			'audio/mp4;codecs=opus',
			'audio/mp4',
			'audio/aac'
		];
		if (typeof MediaRecorder === 'undefined' || !MediaRecorder.isTypeSupported) return undefined;
		return types.find((t) => MediaRecorder.isTypeSupported(t));
	}

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

	// Broadcast-quality capture: disable the browser's voice-processing (echo
	// cancellation / noise suppression / auto gain). It mangles music, and on
	// macOS it forces the audio hardware into "communication" mode — which is what
	// briefly glitches the machine's other audio when the mic opens.
	function micConstraints(deviceId?: string): MediaStreamConstraints {
		const audio: MediaTrackConstraints = {
			echoCancellation: false,
			noiseSuppression: false,
			autoGainControl: false
		};
		if (deviceId) audio.deviceId = { exact: deviceId };
		return { audio };
	}

	// Browsers hide device labels/ids until mic access is granted at least once.
	// Probe for permission, then re-enumerate so the real input names show up.
	async function requestMicAccess() {
		error = '';
		try {
			const probe = await navigator.mediaDevices.getUserMedia(micConstraints());
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
			micStream = await navigator.mediaDevices.getUserMedia(micConstraints(selectedDeviceId || undefined));
			refreshDevices();
			audioCtx = newAudioContext();
			const src = audioCtx.createMediaStreamSource(micStream);
			const an = audioCtx.createAnalyser();
			an.fftSize = 256;
			src.connect(an);
			analyser = an;
			const proto = location.protocol === 'https:' ? 'wss' : 'ws';
			ws = new WebSocket(`${proto}://${location.host}/api/live/ingest?token=${getToken()}`);
			ws.binaryType = 'arraybuffer';
			ws.onopen = () => {
				const mime = pickRecorderMime();
				recorder = new MediaRecorder(micStream!, {
					...(mime ? { mimeType: mime } : {}),
					audioBitsPerSecond: 128000
				});
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

		// No explicit /live/stop call here: closing the socket makes the server
		// drain ffmpeg's buffer in-order (after the final chunk is written), so the
		// tail of the take airs in full. A POST here could race ahead and close the
		// decoder's stdin early, cutting the last words.
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

	// ---- Live chat ----
	let chatMessages = $state<ChatMessage[]>([]);
	let chatConnected = $state(false);
	let chatBody = $state('');
	let chatWs: WebSocket | null = null;
	let chatReconnect: ReturnType<typeof setTimeout> | null = null;
	let chatDestroyed = false;
	let chatScroller: HTMLDivElement | null = null;

	async function chatScrollToBottom() {
		await tick();
		if (chatScroller) chatScroller.scrollTop = chatScroller.scrollHeight;
	}

	function handleChatEvent(ev: ChatEvent) {
		switch (ev.type) {
			case 'message':
				chatMessages = [...chatMessages, ev.message];
				chatScrollToBottom();
				break;
			case 'delete':
				chatMessages = chatMessages.filter((m) => m.id !== ev.id);
				break;
			// 'listeners' / 'error' — nothing to do on the on-air page.
		}
	}

	function connectChat() {
		const url = chatWsUrl();
		if (!url) return;
		try {
			chatWs = new WebSocket(url);
		} catch {
			return;
		}
		chatWs.onopen = () => (chatConnected = true);
		chatWs.onmessage = (e) => {
			try {
				handleChatEvent(JSON.parse(e.data) as ChatEvent);
			} catch {
				// ignore malformed frames
			}
		};
		chatWs.onclose = () => {
			chatConnected = false;
			if (chatDestroyed) return;
			if (chatReconnect) clearTimeout(chatReconnect);
			chatReconnect = setTimeout(connectChat, 2000);
		};
	}

	function send() {
		const body = chatBody.trim();
		if (!body || !chatWs || chatWs.readyState !== WebSocket.OPEN) return;
		chatWs.send(JSON.stringify({ type: 'message', name: 'Régie', body }));
		chatBody = '';
	}

	function onChatKey(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			send();
		}
	}

	function formatChatTime(iso: string): string {
		return new Date(iso).toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' });
	}

	onMount(() => {
		refreshNow();
		nowTimer = setInterval(refreshNow, 5000);
		refreshDevices();
		navigator.mediaDevices?.addEventListener?.('devicechange', refreshDevices);

		api
			.chatHistory()
			.then((hist) => {
				chatMessages = hist;
				chatScrollToBottom();
			})
			.catch(() => {
				/* ignore — history is best-effort */
			})
			.finally(connectChat);
	});

	onDestroy(() => {
		teardownLive();
		clearInterval(nowTimer);
		clearTimeout(clipTimer);
		navigator.mediaDevices?.removeEventListener?.('devicechange', refreshDevices);

		chatDestroyed = true;
		if (chatReconnect) clearTimeout(chatReconnect);
		chatWs?.close();
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

		{#if live}
			<p class="flex items-center gap-1.5 text-center text-xs text-muted-foreground">
				<Icon icon="lucide:headphones" width={14} class="shrink-0" />
				N'écoute pas la radio ici pendant que tu diffuses : tu t'entendrais en différé
				(latence de diffusion normale ~10&nbsp;s). Utilise le monitoring de ton système.
			</p>
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

	<!-- Live chat -->
	<Card class="mt-6 flex flex-col p-0">
		<div class="flex items-center gap-2 border-b border-border px-4 py-3 sm:px-5">
			<Icon icon="lucide:message-circle-more" width={18} class="shrink-0 text-foreground" />
			<h2 class="text-base font-medium text-foreground">Chat en direct</h2>
			<span
				class="ml-auto h-2 w-2 shrink-0 rounded-full {chatConnected
					? 'bg-green-500'
					: 'bg-muted-foreground'}"
				title={chatConnected ? 'Connecté' : 'Déconnecté'}
				aria-hidden="true"
			></span>
		</div>

		<div bind:this={chatScroller} class="max-h-72 flex-1 overflow-y-auto px-4 py-4 sm:px-5">
			{#if chatMessages.length === 0}
				<p class="py-8 text-center text-sm text-muted-foreground">Aucun message pour l'instant.</p>
			{:else}
				<ul class="space-y-3">
					{#each chatMessages as msg (msg.id)}
						<li class="min-w-0">
							<div class="flex flex-wrap items-baseline gap-x-2">
								<span class="text-sm font-bold text-foreground">{msg.name}</span>
								<span class="text-[11px] text-muted-foreground">{formatChatTime(msg.createdAt)}</span>
							</div>
							<p class="mt-0.5 break-words text-sm text-foreground">{msg.body}</p>
						</li>
					{/each}
				</ul>
			{/if}
		</div>

		<div class="border-t border-border p-3">
			<div class="flex items-end gap-2">
				<Input
					bind:value={chatBody}
					onkeydown={onChatKey}
					placeholder="Répondre en tant que Régie…"
					aria-label="Message de la régie"
				/>
				<Button
					size="icon"
					class="shrink-0"
					onclick={send}
					disabled={!chatBody.trim() || !chatConnected}
					aria-label="Envoyer"
				>
					<Icon icon="lucide:send" width={18} />
				</Button>
			</div>
		</div>
	</Card>
</div>
