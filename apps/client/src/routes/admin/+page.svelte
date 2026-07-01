<script lang="ts">
	import { onMount } from 'svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import { api, getToken, setToken, clearToken, type Track } from '$lib/api';

	let loggedIn = $state(false);
	let username = $state('');
	let password = $state('');
	let error = $state('');

	let tracks = $state<Track[]>([]);
	let file = $state<FileList | null>(null);
	let title = $state('');
	let artist = $state('');
	let uploading = $state(false);

	// Live broadcast
	let live = $state(false);
	let ws: WebSocket | null = null;
	let recorder: MediaRecorder | null = null;
	let micStream: MediaStream | null = null;

	async function login() {
		error = '';
		try {
			const res = await api.login(username, password);
			setToken(res.token);
			loggedIn = true;
			await loadTracks();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	function logout() {
		clearToken();
		loggedIn = false;
	}

	async function loadTracks() {
		try {
			tracks = await api.tracks();
		} catch {
			logout();
		}
	}

	async function doUpload(e: SubmitEvent) {
		e.preventDefault();
		if (!file?.[0]) return;
		uploading = true;
		error = '';
		try {
			await api.upload(file[0], title, artist);
			title = '';
			artist = '';
			file = null;
			(e.target as HTMLFormElement).reset();
			await loadTracks();
		} catch (err) {
			error = (err as Error).message;
		} finally {
			uploading = false;
		}
	}

	async function del(id: string) {
		await api.deleteTrack(id);
		await loadTracks();
	}

	async function startLive() {
		error = '';
		try {
			micStream = await navigator.mediaDevices.getUserMedia({ audio: true });
			const proto = location.protocol === 'https:' ? 'wss' : 'ws';
			ws = new WebSocket(`${proto}://${location.host}/api/live/ingest?token=${getToken()}`);
			ws.binaryType = 'arraybuffer';
			ws.onopen = () => {
				recorder = new MediaRecorder(micStream!, { mimeType: 'audio/webm;codecs=opus' });
				recorder.ondataavailable = async (ev) => {
					if (ev.data.size > 0 && ws?.readyState === WebSocket.OPEN) {
						ws.send(await ev.data.arrayBuffer());
					}
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
		recorder?.state !== 'inactive' && recorder?.stop();
		micStream?.getTracks().forEach((t) => t.stop());
		ws?.close();
		recorder = null;
		micStream = null;
		ws = null;
		live = false;
	}

	async function stopLive() {
		teardownLive();
		await api.stopLive().catch(() => {});
	}

	onMount(() => {
		if (getToken()) {
			loggedIn = true;
			loadTracks();
		}
	});
</script>

<main class="mx-auto max-w-2xl px-6 py-12">
	<header class="mb-10 flex items-center justify-between">
		<a href="/" class="flex items-center gap-2">
			<Icon icon="solar:podcast-bold-duotone" width={32} />
			<span class="text-xl font-bold">Antenne <span class="text-[var(--color-muted-foreground)]">· admin</span></span>
		</a>
		{#if loggedIn}
			<Button variant="ghost" size="sm" onclick={logout}>Déconnexion</Button>
		{/if}
	</header>

	{#if !loggedIn}
		<Card class="mx-auto max-w-sm">
			<h2 class="mb-4 text-lg font-semibold">Connexion</h2>
			<div class="flex flex-col gap-3">
				<Input placeholder="Identifiant" bind:value={username} />
				<Input type="password" placeholder="Mot de passe" bind:value={password} />
				{#if error}<p class="text-sm text-red-500">{error}</p>{/if}
				<Button onclick={login}>Entrer en régie</Button>
			</div>
		</Card>
	{:else}
		<!-- Prise d'antenne -->
		<Card class="mb-8">
			<div class="flex items-center justify-between">
				<div>
					<h2 class="text-lg font-semibold">Prise d'antenne</h2>
					<p class="text-sm text-[var(--color-muted-foreground)]">
						{live ? 'Tu es en direct par-dessus la playlist.' : 'La playlist tourne. Prends le micro pour passer en direct.'}
					</p>
				</div>
				{#if live}
					<Button variant="destructive" onclick={stopLive}>
						<Icon icon="solar:stop-bold" width={20} /> Rendre l'antenne
					</Button>
				{:else}
					<Button onclick={startLive}>
						<Icon icon="solar:microphone-3-bold-duotone" width={20} /> Prendre le micro
					</Button>
				{/if}
			</div>
		</Card>

		<!-- Upload -->
		<Card class="mb-8">
			<h2 class="mb-4 text-lg font-semibold">Ajouter un son</h2>
			<form class="flex flex-col gap-3" onsubmit={doUpload}>
				<Input placeholder="Titre" bind:value={title} />
				<Input placeholder="Artiste" bind:value={artist} />
				<input
					type="file"
					accept="audio/*"
					onchange={(e) => (file = (e.target as HTMLInputElement).files)}
					class="text-sm file:mr-3 file:rounded-[var(--radius)] file:border file:border-[var(--color-border)] file:bg-transparent file:px-3 file:py-1.5 file:text-[var(--color-foreground)]"
				/>
				<Button type="submit" disabled={uploading}>
					<Icon icon="solar:upload-bold-duotone" width={20} />
					{uploading ? 'Envoi…' : 'Uploader'}
				</Button>
			</form>
		</Card>

		<!-- Playlist -->
		<Card>
			<h2 class="mb-4 text-lg font-semibold">Playlist ({tracks.length})</h2>
			{#if tracks.length === 0}
				<p class="text-sm text-[var(--color-muted-foreground)]">Aucun son pour l'instant.</p>
			{:else}
				<ul class="flex flex-col divide-y divide-[var(--color-border)]">
					{#each tracks as t (t.id)}
						<li class="flex items-center justify-between py-3">
							<div class="flex items-center gap-3">
								<Icon icon="solar:soundwave-bold-duotone" width={22} />
								<div>
									<p class="font-medium">{t.title}</p>
									{#if t.artist}<p class="text-xs text-[var(--color-muted-foreground)]">{t.artist}</p>{/if}
								</div>
							</div>
							<Button variant="ghost" size="icon" onclick={() => del(t.id)}>
								<Icon icon="solar:trash-bin-trash-bold-duotone" width={20} />
							</Button>
						</li>
					{/each}
				</ul>
			{/if}
		</Card>

		{#if error}<p class="mt-4 text-sm text-red-500">{error}</p>{/if}
	{/if}
</main>
