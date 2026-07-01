<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import { api, chatWsUrl, type ChatMessage, type ChatEvent } from '$lib/api';

	// Floating, draggable, minimisable live-chat window for listeners.
	const NAME_KEY = 'antenne_chatname';
	const POS_KEY = 'antenne_chatpos';

	let open = $state(true);
	let name = $state('');
	let body = $state('');
	let messages = $state<ChatMessage[]>([]);
	let chatListeners = $state(0);
	let chatError = $state('');
	let editingName = $state(false);

	let ws: WebSocket | null = null;
	let wsOpen = $state(false);
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let errorTimer: ReturnType<typeof setTimeout> | null = null;
	let closed = false;
	let listEl: HTMLDivElement | undefined = $state();
	let atBottom = true;

	// Drag (translate from the bottom-right anchor).
	let dx = $state(0);
	let dy = $state(0);
	let dragging = false;
	let sx = 0,
		sy = 0,
		ox = 0,
		oy = 0;

	const canSend = $derived(!!name.trim() && !!body.trim() && wsOpen);

	function startDrag(e: PointerEvent) {
		dragging = true;
		sx = e.clientX;
		sy = e.clientY;
		ox = dx;
		oy = dy;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
	}
	function onDrag(e: PointerEvent) {
		if (!dragging) return;
		dx = ox + (e.clientX - sx);
		dy = oy + (e.clientY - sy);
	}
	function endDrag() {
		if (!dragging) return;
		dragging = false;
		try {
			localStorage.setItem(POS_KEY, JSON.stringify({ dx, dy }));
		} catch {
			/* ignore */
		}
	}

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
				/* ignore */
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
		try {
			const p = JSON.parse(localStorage.getItem(POS_KEY) ?? 'null');
			if (p && typeof p.dx === 'number') {
				dx = p.dx;
				dy = p.dy;
			}
		} catch {
			/* ignore */
		}
		(async () => {
			try {
				messages = await api.chatHistory();
				scrollToBottom();
			} catch {
				/* empty */
			}
			connect();
		})();
	});

	onDestroy(() => {
		closed = true;
		if (reconnectTimer) clearTimeout(reconnectTimer);
		if (errorTimer) clearTimeout(errorTimer);
		ws?.close();
	});
</script>

<div
	class="fixed bottom-4 right-4 z-50"
	style="transform: translate({dx}px, {dy}px)"
>
	{#if open}
		<div
			class="flex h-[28rem] w-[20rem] max-w-[calc(100vw-2rem)] flex-col overflow-hidden rounded-2xl border border-border/60 bg-background/85 shadow-2xl shadow-black/40 ring-1 ring-white/5 backdrop-blur-xl"
		>
			<!-- Draggable header -->
			<div
				onpointerdown={startDrag}
				onpointermove={onDrag}
				onpointerup={endDrag}
				onpointercancel={endDrag}
				class="flex cursor-grab items-center justify-between border-b border-border/60 px-3 py-2.5 select-none active:cursor-grabbing"
			>
				<div class="flex items-center gap-2 text-sm font-medium">
					<Icon icon="solar:chat-round-dots-bold-duotone" width={18} />
					<span>Chat en direct</span>
				</div>
				<div class="flex items-center gap-2">
					<span class="flex items-center gap-1 text-xs text-[var(--color-muted-foreground)]">
						<span class="h-1.5 w-1.5 rounded-full {wsOpen ? 'bg-green-500' : 'bg-[var(--color-muted-foreground)]'}"></span>
						{chatListeners}
					</span>
					<button
						onclick={() => (open = false)}
						aria-label="Minimiser"
						class="text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]"
					>
						<Icon icon="solar:minimize-square-linear" width={18} />
					</button>
				</div>
			</div>

			<!-- Messages -->
			<div
				bind:this={listEl}
				onscroll={onScroll}
				class="min-h-0 flex-1 overflow-y-auto p-3"
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
								<span class="ml-1 text-[10px] text-[var(--color-muted-foreground)]">{fmtTime(msg.createdAt)}</span>
								<p class="break-words text-[var(--color-foreground)]">{msg.body}</p>
							</li>
						{/each}
					</ul>
				{/if}
			</div>

			{#if chatError}
				<p class="px-3 text-xs text-red-500">{chatError}</p>
			{/if}

			<!-- Composer -->
			<div class="border-t border-border/60 p-3">
				{#if editingName}
					<div class="flex gap-2">
						<Input
							bind:value={name}
							placeholder="Ton pseudo"
							maxlength={32}
							onkeydown={(e: KeyboardEvent) => e.key === 'Enter' && saveName()}
						/>
						<Button variant="outline" size="sm" onclick={saveName} disabled={!name.trim()}>OK</Button>
					</div>
				{:else}
					<div class="mb-2 flex items-center justify-between text-xs text-[var(--color-muted-foreground)]">
						<span>Pseudo : <span class="font-medium text-[var(--color-foreground)]">{name}</span></span>
						<button class="underline hover:no-underline" onclick={() => (editingName = true)}>changer</button>
					</div>
					<div class="flex gap-2">
						<Input bind:value={body} placeholder="Ton message..." maxlength={500} onkeydown={onChatKey} />
						<Button size="icon" onclick={send} disabled={!canSend} aria-label="Envoyer">
							<Icon icon="solar:plain-2-bold" width={18} />
						</Button>
					</div>
				{/if}
			</div>
		</div>
	{:else}
		<!-- Minimised launcher -->
		<button
			onclick={() => (open = true)}
			class="flex items-center gap-2 rounded-full border border-border/60 bg-background/85 px-4 py-2.5 text-sm font-medium shadow-2xl shadow-black/40 ring-1 ring-white/5 backdrop-blur-xl hover:bg-muted/60"
		>
			<Icon icon="solar:chat-round-dots-bold-duotone" width={18} />
			Chat
			<span class="flex items-center gap-1 text-xs text-[var(--color-muted-foreground)]">
				<span class="h-1.5 w-1.5 rounded-full {wsOpen ? 'bg-green-500' : 'bg-[var(--color-muted-foreground)]'}"></span>
				{chatListeners}
			</span>
		</button>
	{/if}
</div>
