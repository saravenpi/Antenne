<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import {
		api,
		chatWsUrl,
		type ChatMessage,
		type ChatEvent,
		type Ban,
		type Restriction
	} from '$lib/api';
	import Icon from '$lib/components/Icon.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Input from '$lib/components/ui/Input.svelte';

	// ---- Live chat state ----
	let messages = $state<ChatMessage[]>([]);
	let listeners = $state(0);
	let connected = $state(false);
	let wsError = $state<string | null>(null);
	let composeBody = $state('');
	let scroller: HTMLDivElement | null = null;
	let ws: WebSocket | null = null;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let destroyed = false;

	async function scrollToBottom() {
		await tick();
		if (scroller) scroller.scrollTop = scroller.scrollHeight;
	}

	function handleEvent(ev: ChatEvent) {
		switch (ev.type) {
			case 'message':
				messages = [...messages, ev.message];
				scrollToBottom();
				break;
			case 'delete':
				messages = messages.filter((m) => m.id !== ev.id);
				break;
			case 'listeners':
				listeners = ev.count;
				break;
			case 'error':
				wsError = ev.error;
				break;
		}
	}

	function connect() {
		const url = chatWsUrl();
		if (!url) return;
		try {
			ws = new WebSocket(url);
		} catch (err) {
			wsError = err instanceof Error ? err.message : 'Connexion impossible';
			return;
		}
		ws.onopen = () => {
			connected = true;
			wsError = null;
		};
		ws.onmessage = (e) => {
			try {
				handleEvent(JSON.parse(e.data) as ChatEvent);
			} catch {
				// ignore malformed frames
			}
		};
		ws.onclose = () => {
			connected = false;
			if (destroyed) return;
			if (reconnectTimer) clearTimeout(reconnectTimer);
			reconnectTimer = setTimeout(connect, 2000);
		};
		ws.onerror = () => {
			wsError = 'Erreur de connexion au chat.';
		};
	}

	function send() {
		const body = composeBody.trim();
		if (!body || !ws || ws.readyState !== WebSocket.OPEN) return;
		ws.send(JSON.stringify({ type: 'message', name: 'Régie', body }));
		composeBody = '';
	}

	function onComposeKey(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			send();
		}
	}

	// ---- Per-message moderation actions ----
	async function deleteMessage(id: string) {
		try {
			await api.deleteMessage(id);
			messages = messages.filter((m) => m.id !== id);
		} catch (err) {
			wsError = err instanceof Error ? err.message : 'Suppression impossible';
		}
	}

	async function banFromMessage(ip: string) {
		try {
			await api.ban(ip);
			await loadBans();
		} catch (err) {
			wsError = err instanceof Error ? err.message : 'Bannissement impossible';
		}
	}

	async function restrictFromMessage(ip: string) {
		try {
			await api.restrict(ip);
			await loadRestrictions();
		} catch (err) {
			wsError = err instanceof Error ? err.message : 'Restriction impossible';
		}
	}

	function formatTime(iso: string): string {
		return new Date(iso).toLocaleTimeString('fr-FR');
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString('fr-FR');
	}

	// ---- Bans panel ----
	let bans = $state<Ban[]>([]);
	let banIp = $state('');
	let banReason = $state('');
	let banBusy = $state(false);
	let banError = $state<string | null>(null);

	async function loadBans() {
		try {
			bans = await api.bans();
		} catch (err) {
			banError = err instanceof Error ? err.message : 'Chargement impossible';
		}
	}

	async function addBan(e: SubmitEvent) {
		e.preventDefault();
		const ip = banIp.trim();
		if (!ip) return;
		banBusy = true;
		banError = null;
		try {
			await api.ban(ip, banReason.trim() || undefined);
			banIp = '';
			banReason = '';
			await loadBans();
		} catch (err) {
			banError = err instanceof Error ? err.message : 'Bannissement impossible';
		} finally {
			banBusy = false;
		}
	}

	async function removeBan(id: string) {
		try {
			await api.unban(id);
			await loadBans();
		} catch (err) {
			banError = err instanceof Error ? err.message : 'Débannissement impossible';
		}
	}

	// ---- Restrictions panel ----
	let restrictions = $state<Restriction[]>([]);
	let restrictIp = $state('');
	let restrictReason = $state('');
	let restrictBusy = $state(false);
	let restrictError = $state<string | null>(null);

	async function loadRestrictions() {
		try {
			restrictions = await api.restrictions();
		} catch (err) {
			restrictError = err instanceof Error ? err.message : 'Chargement impossible';
		}
	}

	async function addRestriction(e: SubmitEvent) {
		e.preventDefault();
		const ip = restrictIp.trim();
		if (!ip) return;
		restrictBusy = true;
		restrictError = null;
		try {
			await api.restrict(ip, restrictReason.trim() || undefined);
			restrictIp = '';
			restrictReason = '';
			await loadRestrictions();
		} catch (err) {
			restrictError = err instanceof Error ? err.message : 'Restriction impossible';
		} finally {
			restrictBusy = false;
		}
	}

	async function removeRestriction(id: string) {
		try {
			await api.unrestrict(id);
			await loadRestrictions();
		} catch (err) {
			restrictError = err instanceof Error ? err.message : 'Levée impossible';
		}
	}

	// ---- Settings panel ----
	let bannedWordsText = $state('');
	let slowModeSec = $state(0);
	let settingsBusy = $state(false);
	let settingsSaved = $state(false);
	let settingsError = $state<string | null>(null);
	let savedTimer: ReturnType<typeof setTimeout> | null = null;

	async function loadSettings() {
		try {
			const s = await api.settings();
			bannedWordsText = s.bannedWords.join('\n');
			slowModeSec = s.slowModeSec ?? 0;
		} catch (err) {
			settingsError = err instanceof Error ? err.message : 'Chargement impossible';
		}
	}

	async function saveSettings(e: SubmitEvent) {
		e.preventDefault();
		settingsBusy = true;
		settingsError = null;
		settingsSaved = false;
		try {
			await api.saveSettings({
				bannedWords: bannedWordsText
					.split('\n')
					.map((s) => s.trim())
					.filter(Boolean),
				slowModeSec: Math.max(0, Math.floor(Number(slowModeSec) || 0))
			});
			settingsSaved = true;
			if (savedTimer) clearTimeout(savedTimer);
			savedTimer = setTimeout(() => (settingsSaved = false), 2500);
		} catch (err) {
			settingsError = err instanceof Error ? err.message : 'Enregistrement impossible';
		} finally {
			settingsBusy = false;
		}
	}

	onMount(async () => {
		try {
			const hist = await api.chatHistory();
			messages = hist;
			scrollToBottom();
		} catch (err) {
			wsError = err instanceof Error ? err.message : "Impossible de charger l'historique";
		}
		connect();
		await Promise.all([loadBans(), loadRestrictions(), loadSettings()]);
	});

	onDestroy(() => {
		destroyed = true;
		if (reconnectTimer) clearTimeout(reconnectTimer);
		if (savedTimer) clearTimeout(savedTimer);
		ws?.close();
	});
</script>

<div class="mx-auto max-w-4xl px-6 py-10 md:px-10">
	<div class="mb-8">
		<h1 class="text-2xl font-semibold tracking-tight text-foreground">Chat &amp; modération</h1>
		<p class="mt-1 text-sm text-muted-foreground">
			Le direct de la régie : suivez les messages, répondez, et modérez les auditeurs.
		</p>
	</div>

	<div class="grid grid-cols-1 gap-6 lg:grid-cols-5">
		<!-- ================= Live chat ================= -->
		<div class="lg:col-span-3">
			<Card class="flex flex-col p-0">
				<div class="flex items-center justify-between gap-3 border-b border-border px-5 py-3">
					<div class="flex items-center gap-2">
						<span
							class="h-2 w-2 rounded-full {connected ? 'bg-green-500' : 'bg-muted-foreground'}"
							aria-hidden="true"
						></span>
						<span class="text-sm font-medium text-foreground">
							{connected ? 'En direct' : 'Déconnecté'}
						</span>
					</div>
					<span class="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
						<Icon icon="solar:users-group-rounded-linear" width={16} />
						{listeners} à l'écoute
					</span>
				</div>

				{#if wsError}
					<div
						class="border-b border-border bg-muted px-5 py-2 text-xs text-muted-foreground"
						role="alert"
					>
						{wsError}
					</div>
				{/if}

				<div bind:this={scroller} class="max-h-[28rem] min-h-[16rem] overflow-y-auto px-5 py-4">
					{#if messages.length === 0}
						<p class="py-12 text-center text-sm text-muted-foreground">Aucun message pour l'instant.</p>
					{:else}
						<ul class="space-y-3">
							{#each messages as msg (msg.id)}
								<li class="group flex items-start justify-between gap-3">
									<div class="min-w-0">
										<div class="flex flex-wrap items-baseline gap-x-2">
											<span class="text-sm font-bold text-foreground">{msg.name}</span>
											<span class="text-[11px] text-muted-foreground">{formatTime(msg.createdAt)}</span>
											{#if msg.ip}
												<span class="font-mono text-[11px] text-muted-foreground">{msg.ip}</span>
											{/if}
										</div>
										<p class="mt-0.5 break-words text-sm text-foreground">{msg.body}</p>
									</div>
									<div
										class="flex shrink-0 items-center gap-0.5 opacity-0 transition group-hover:opacity-100"
									>
										<Button
											variant="ghost"
											size="icon"
											class="h-8 w-8"
											onclick={() => deleteMessage(msg.id)}
											aria-label="Supprimer le message"
											title="Supprimer"
										>
											<Icon icon="solar:trash-bin-trash-linear" width={16} />
										</Button>
										{#if msg.ip}
											{@const ip = msg.ip}
											<Button
												variant="ghost"
												size="icon"
												class="h-8 w-8"
												onclick={() => banFromMessage(ip)}
												aria-label="Bannir l'IP"
												title="Bannir l'IP"
											>
												<Icon icon="solar:forbidden-circle-linear" width={16} />
											</Button>
											<Button
												variant="ghost"
												size="icon"
												class="h-8 w-8"
												onclick={() => restrictFromMessage(ip)}
												aria-label="Restreindre l'IP"
												title="Restreindre (shadow-ban)"
											>
												<Icon icon="solar:eye-closed-linear" width={16} />
											</Button>
										{/if}
									</div>
								</li>
							{/each}
						</ul>
					{/if}
				</div>

				<div class="border-t border-border p-3">
					<div class="flex items-end gap-2">
						<Input
							bind:value={composeBody}
							onkeydown={onComposeKey}
							placeholder="Répondre en tant que Régie…"
							aria-label="Message de la régie"
						/>
						<Button size="icon" onclick={send} disabled={!composeBody.trim() || !connected} aria-label="Envoyer">
							<Icon icon="solar:plain-linear" width={18} />
						</Button>
					</div>
				</div>
			</Card>
		</div>

		<!-- ================= Moderation column ================= -->
		<div class="flex flex-col gap-6 lg:col-span-2">
			<!-- Bans -->
			<Card>
				<div class="mb-3 flex items-center gap-2">
					<Icon icon="solar:forbidden-circle-linear" width={18} class="text-foreground" />
					<h2 class="text-base font-semibold text-foreground">Bannis (IP)</h2>
				</div>

				<form class="mb-4 space-y-2" onsubmit={addBan}>
					<Input bind:value={banIp} placeholder="Adresse IP" aria-label="IP à bannir" />
					<Input bind:value={banReason} placeholder="Raison (optionnel)" aria-label="Raison du bannissement" />
					<Button type="submit" size="sm" class="w-full" disabled={banBusy || !banIp.trim()}>
						Bannir
					</Button>
					{#if banError}
						<p class="text-xs text-red-500">{banError}</p>
					{/if}
				</form>

				{#if bans.length === 0}
					<p class="text-sm text-muted-foreground">Aucun bannissement.</p>
				{:else}
					<ul class="space-y-2">
						{#each bans as b (b.id)}
							<li class="flex items-start justify-between gap-2 border-t border-border pt-2">
								<div class="min-w-0">
									<p class="truncate font-mono text-sm text-foreground">{b.ip}</p>
									{#if b.reason}
										<p class="truncate text-xs text-muted-foreground">{b.reason}</p>
									{/if}
									<p class="text-[11px] text-muted-foreground">{formatDate(b.createdAt)}</p>
								</div>
								<Button variant="outline" size="sm" onclick={() => removeBan(b.id)}>Débannir</Button>
							</li>
						{/each}
					</ul>
				{/if}
			</Card>

			<!-- Restrictions -->
			<Card>
				<div class="mb-1 flex items-center gap-2">
					<Icon icon="solar:eye-closed-linear" width={18} class="text-foreground" />
					<h2 class="text-base font-semibold text-foreground">Restreints (shadow-ban)</h2>
				</div>
				<p class="mb-3 text-xs text-muted-foreground">
					Un utilisateur restreint voit ses messages, mais personne d'autre ne les voit.
				</p>

				<form class="mb-4 space-y-2" onsubmit={addRestriction}>
					<Input bind:value={restrictIp} placeholder="Adresse IP" aria-label="IP à restreindre" />
					<Input
						bind:value={restrictReason}
						placeholder="Raison (optionnel)"
						aria-label="Raison de la restriction"
					/>
					<Button type="submit" size="sm" class="w-full" disabled={restrictBusy || !restrictIp.trim()}>
						Restreindre
					</Button>
					{#if restrictError}
						<p class="text-xs text-red-500">{restrictError}</p>
					{/if}
				</form>

				{#if restrictions.length === 0}
					<p class="text-sm text-muted-foreground">Aucune restriction.</p>
				{:else}
					<ul class="space-y-2">
						{#each restrictions as r (r.id)}
							<li class="flex items-start justify-between gap-2 border-t border-border pt-2">
								<div class="min-w-0">
									<p class="truncate font-mono text-sm text-foreground">{r.ip}</p>
									{#if r.reason}
										<p class="truncate text-xs text-muted-foreground">{r.reason}</p>
									{/if}
									<p class="text-[11px] text-muted-foreground">{formatDate(r.createdAt)}</p>
								</div>
								<Button variant="outline" size="sm" onclick={() => removeRestriction(r.id)}>Lever</Button>
							</li>
						{/each}
					</ul>
				{/if}
			</Card>
		</div>
	</div>

	<!-- ================= Settings ================= -->
	<Card class="mt-6">
		<div class="mb-3 flex items-center gap-2">
			<Icon icon="solar:settings-linear" width={18} class="text-foreground" />
			<h2 class="text-base font-semibold text-foreground">Réglages du chat</h2>
		</div>

		<form class="grid grid-cols-1 gap-4 md:grid-cols-2" onsubmit={saveSettings}>
			<div>
				<label for="banned-words" class="mb-1.5 block text-sm font-medium text-foreground">
					Mots interdits
				</label>
				<textarea
					id="banned-words"
					bind:value={bannedWordsText}
					rows="6"
					placeholder="Un mot interdit par ligne"
					class="w-full rounded-[var(--radius)] border border-border bg-transparent px-3 py-2 text-sm outline-none placeholder:text-muted-foreground focus:ring-2 focus:ring-[var(--color-foreground)]/20"
				></textarea>
				<p class="mt-1.5 text-xs text-muted-foreground">
					Un mot par ligne. Liste vide par défaut = aucun filtrage.
				</p>
			</div>

			<div>
				<label for="slow-mode" class="mb-1.5 block text-sm font-medium text-foreground">
					Mode lent (secondes)
				</label>
				<Input
					id="slow-mode"
					type="number"
					min="0"
					bind:value={slowModeSec}
					placeholder="0"
					aria-label="Délai minimum entre messages"
				/>
				<p class="mt-1.5 text-xs text-muted-foreground">Délai minimum entre deux messages.</p>
			</div>

			<div class="flex items-center gap-3 md:col-span-2">
				<Button type="submit" disabled={settingsBusy}>Enregistrer</Button>
				{#if settingsSaved}
					<span class="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
						<Icon icon="solar:check-circle-linear" width={16} />
						Enregistré
					</span>
				{/if}
				{#if settingsError}
					<span class="text-sm text-red-500">{settingsError}</span>
				{/if}
			</div>
		</form>
	</Card>
</div>
