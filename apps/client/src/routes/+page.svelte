<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import Hls from 'hls.js';
	import Icon from '$lib/components/Icon.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import AudioVisualizer from '$lib/components/AudioVisualizer.svelte';
	import { api, chatWsUrl, type NowPlaying, type ChatMessage, type ChatEvent } from '$lib/api';

	// ---- Player ----
	let audio: HTMLAudioElement;
	let hls: Hls | null = null;
	let playing = $state(false);
	let np = $state<NowPlaying | null>(null);
	let audioCtx: AudioContext | null = null;
	let analyser = $state<AnalyserNode | null>(null);
	let graphReady = false;
	let poll: ReturnType<typeof setInterval>;

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
		const src = audioCtx.createMediaElementSource(audio); // once only!
		const an = audioCtx.createAnalyser();
		an.fftSize = 512;
		src.connect(an);
		an.connect(audioCtx.destination); // keep audio audible
		analyser = an;
		graphReady = true;
	}

	async function toggle() {
		if (playing) {
			audio.pause();
			playing = false;
			return;
		}
		if (!hls && !audio.src) attach();
		setupGraph();
		await audioCtx?.resume();
		await audio.play();
		playing = true;
	}

	async function refresh() {
		try {
			// `playingDate` reflects the wall-clock at the current playback position
			// (from PROGRAM-DATE-TIME) so the title matches what the listener hears.
			const pd = hls?.playingDate;
			const at = pd ? pd.getTime() : undefined;
			// api.nowPlaying accepts an optional wall-clock (unix ms).
			np = await (api.nowPlaying as (atMillis?: number) => Promise<NowPlaying>)(at);
		} catch {
			/* keep last */
		}
	}

	// ---- Chat ----
	const NAME_KEY = 'antenne_chatname';
	let name = $state('');
	let body = $state('');
	let messages = $state<ChatMessage[]>([]);
	let chatListeners = $state(0);
	let chatError = $state('');
	let editingName = $state(false);
	let ws: WebSocket | null = null;
	let wsOpen = $state(false);
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let closed = false;
	let errorTimer: ReturnType<typeof setTimeout> | null = null;
	let listEl: HTMLDivElement | undefined = $state();
	let atBottom = true;

	const canSend = $derived(!!name.trim() && !!body.trim() && wsOpen);

	function saveName() {
		const n = name.trim();
		if (!n) return;
		localStorage.setItem(NAME_KEY, n);
		editingName = false;
	}

	function showError(msg: string) {
		chatError = msg;
		if (errorTimer) clearTimeout(errorTimer);
		errorTimer = setTimeout(() => (chatError = ''), 5000);
	}

	function onScroll() {
		if (!listEl) return;
		atBottom = listEl.scrollHeight - listEl.scrollTop - listEl.clientHeight < 40;
	}

	async function scrollToBottom() {
		await tick();
		if (listEl) listEl.scrollTop = listEl.scrollHeight;
	}

	function handleEvent(ev: ChatEvent) {
		if (ev.type === 'message') {
			messages = [...messages, ev.message].slice(-200);
			if (atBottom) scrollToBottom();
		} else if (ev.type === 'delete') {
			messages = messages.filter((m) => m.id !== ev.id);
		} else if (ev.type === 'listeners') {
			chatListeners = ev.count;
		} else if (ev.type === 'error') {
			showError(ev.error);
		}
	}

	function connect() {
		if (closed) return;
		ws = new WebSocket(chatWsUrl());
		ws.onopen = () => (wsOpen = true);
		ws.onmessage = (e) => {
			try {
				handleEvent(JSON.parse(e.data) as ChatEvent);
			} catch {
				/* ignore malformed */
			}
		};
		ws.onclose = () => {
			wsOpen = false;
			ws = null;
			if (!closed) reconnectTimer = setTimeout(connect, 2500);
		};
		ws.onerror = () => ws?.close();
	}

	function send() {
		if (!canSend || !ws) return;
		ws.send(JSON.stringify({ type: 'message', name: name.trim(), body: body.trim() }));
		body = '';
	}

	function onChatKey(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			send();
		}
	}

	function fmtTime(iso: string) {
		try {
			return new Date(iso).toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' });
		} catch {
			return '';
		}
	}

	onMount(() => {
		name = localStorage.getItem(NAME_KEY) ?? '';
		editingName = !name;

		refresh();
		poll = setInterval(refresh, 4000);

		(async () => {
			try {
				messages = await api.chatHistory();
				scrollToBottom();
			} catch {
				/* start empty */
			}
			connect();
		})();
	});

	onDestroy(() => {
		clearInterval(poll);
		closed = true;
		if (reconnectTimer) clearTimeout(reconnectTimer);
		if (errorTimer) clearTimeout(errorTimer);
		ws?.close();
		hls?.destroy();
		audioCtx?.close();
	});
</script>

<audio bind:this={audio} class="hidden"></audio>

<main
	class="mx-auto grid min-h-screen max-w-5xl grid-cols-1 items-center gap-10 px-6 py-12 lg:grid-cols-[1fr_22rem] lg:gap-14"
>
	<!-- Station + player -->
	<section class="flex flex-col items-center gap-10">
		<div class="flex flex-col items-center gap-3">
			<Icon icon="solar:podcast-bold-duotone" width={64} />
			<h1 class="text-4xl font-bold tracking-tight">Antenne</h1>
		</div>

		<!-- Radial visualizer wrapping the play button -->
		<div class="relative h-56 w-56">
			<AudioVisualizer {analyser} variant="radial" class="absolute inset-0 h-full w-full" />
			<div class="absolute inset-0 flex items-center justify-center">
				<Button
					size="lg"
					class="h-20 w-20 rounded-full"
					onclick={toggle}
					aria-label={playing ? 'Pause' : 'Lecture'}
				>
					<Icon icon={playing ? 'solar:pause-bold' : 'solar:play-bold'} width={32} />
				</Button>
			</div>
		</div>

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
			{#if np?.next?.title}
				<p class="mt-1 text-xs text-[var(--color-muted-foreground)]">À suivre : {np.next.title}</p>
			{/if}
		</div>

		<div class="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
			<Icon icon="solar:users-group-rounded-bold-duotone" width={18} />
			<span>{np?.listeners ?? 0} à l'écoute</span>
		</div>
	</section>

	<!-- Live chat -->
	<Card class="flex h-full max-h-[36rem] flex-col gap-3 p-4">
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2 text-sm font-medium">
				<Icon icon="solar:chat-round-dots-bold-duotone" width={18} />
				<span>Chat en direct</span>
			</div>
			<span class="flex items-center gap-1 text-xs text-[var(--color-muted-foreground)]">
				<span
					class="h-1.5 w-1.5 rounded-full {wsOpen ? 'bg-green-500' : 'bg-[var(--color-muted-foreground)]'}"
				></span>
				{chatListeners}
			</span>
		</div>

		<!-- Messages -->
		<div
			bind:this={listEl}
			onscroll={onScroll}
			class="flex-1 min-h-0 max-h-64 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-muted)] p-3"
		>
			{#if messages.length === 0}
				<p class="py-6 text-center text-xs text-[var(--color-muted-foreground)]">
					Aucun message. Lance la conversation.
				</p>
			{:else}
				<ul class="flex flex-col gap-2">
					{#each messages as msg (msg.id)}
						<li class="text-sm leading-snug">
							<span class="font-semibold">{msg.name}</span>
							<span class="ml-1 text-[10px] text-[var(--color-muted-foreground)]"
								>{fmtTime(msg.createdAt)}</span
							>
							<p class="break-words text-[var(--color-foreground)]">{msg.body}</p>
						</li>
					{/each}
				</ul>
			{/if}
		</div>

		{#if chatError}
			<p class="text-xs text-red-500">{chatError}</p>
		{/if}

		<!-- Composer -->
		{#if editingName}
			<div class="flex flex-col gap-2">
				<label class="text-xs text-[var(--color-muted-foreground)]" for="chat-name">Ton pseudo</label>
				<div class="flex gap-2">
					<Input
						id="chat-name"
						bind:value={name}
						placeholder="Ton pseudo"
						maxlength={32}
						onkeydown={(e: KeyboardEvent) => e.key === 'Enter' && saveName()}
					/>
					<Button variant="outline" size="sm" onclick={saveName} disabled={!name.trim()}>OK</Button>
				</div>
			</div>
		{:else}
			<div class="flex items-center justify-between text-xs text-[var(--color-muted-foreground)]">
				<span>
					Pseudo : <span class="font-medium text-[var(--color-foreground)]">{name}</span>
				</span>
				<button class="underline hover:no-underline" onclick={() => (editingName = true)}>
					changer
				</button>
			</div>
			<div class="flex gap-2">
				<Input
					bind:value={body}
					placeholder="Ton message..."
					maxlength={500}
					onkeydown={onChatKey}
				/>
				<Button size="icon" onclick={send} disabled={!canSend} aria-label="Envoyer">
					<Icon icon="solar:plain-2-bold" width={18} />
				</Button>
			</div>
		{/if}
	</Card>
</main>
