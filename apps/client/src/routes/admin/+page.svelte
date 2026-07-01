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
	let busy = $state(false);

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

	async function login(e?: SubmitEvent) {
		e?.preventDefault();
		error = '';
		busy = true;
		try {
			const res = await api.login(username, password);
			setToken(res.token);
			loggedIn = true;
			await loadTracks();
		} catch (err) {
			error = (err as Error).message;
		} finally {
			busy = false;
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

	const year = new Date().getFullYear();
</script>

<svelte:head>
	<title>Régie — Antenne</title>
</svelte:head>

{#if !loggedIn}
	<!-- Split-screen login (Nuage style) -->
	<div class="flex min-h-screen">
		<div class="hidden flex-col bg-black px-12 py-10 lg:flex lg:w-1/2">
			<a href="/" class="mb-auto flex items-center gap-3">
				<Icon icon="solar:podcast-bold-duotone" width={28} class="text-white" />
				<span class="text-xl font-bold tracking-tight text-white">Antenne</span>
			</a>

			<div class="mb-auto">
				<h2 class="text-4xl font-bold leading-tight tracking-tight text-white">
					À toi<br />l'antenne.
				</h2>
				<p class="mt-4 max-w-xs text-sm leading-relaxed text-white/50">
					La régie d'Antenne — uploade tes sons, prends le micro et diffuse en direct 24/7.
				</p>
			</div>

			<p class="text-xs text-white/30">© {year} Antenne · Facile Studio</p>
		</div>

		<div class="flex w-full flex-col items-center justify-center bg-background px-8 py-12 lg:w-1/2">
			<div class="w-full max-w-sm">
				<div class="mb-8 flex items-center gap-2 lg:hidden">
					<Icon icon="solar:podcast-bold-duotone" width={26} />
					<span class="text-lg font-bold tracking-tight">Antenne</span>
				</div>

				<div class="mb-8">
					<h1 class="text-2xl font-bold tracking-tight text-foreground">Bienvenue en régie</h1>
					<p class="mt-1.5 text-sm text-muted-foreground">Connecte-toi pour prendre l'antenne.</p>
				</div>

				<form onsubmit={login} class="space-y-4">
					<div class="space-y-1.5">
						<label for="username" class="text-sm font-medium leading-none">Identifiant</label>
						<input
							id="username"
							type="text"
							bind:value={username}
							placeholder="admin"
							required
							autocomplete="username"
							class="flex h-10 w-full rounded-md border border-border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground/20"
						/>
					</div>

					<div class="space-y-1.5">
						<label for="password" class="text-sm font-medium leading-none">Mot de passe</label>
						<input
							id="password"
							type="password"
							bind:value={password}
							placeholder="••••••••"
							required
							autocomplete="current-password"
							class="flex h-10 w-full rounded-md border border-border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground/20"
						/>
					</div>

					{#if error}
						<p class="text-sm text-red-500">{error}</p>
					{/if}

					<button
						type="submit"
						disabled={busy}
						class="inline-flex h-10 w-full items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:opacity-90 disabled:pointer-events-none disabled:opacity-50"
					>
						{busy ? 'Connexion…' : 'Entrer en régie'}
					</button>
				</form>
			</div>
		</div>
	</div>
{:else}
	<main class="mx-auto max-w-2xl px-6 py-12">
		<header class="mb-10 flex items-center justify-between">
			<a href="/" class="flex items-center gap-2">
				<Icon icon="solar:podcast-bold-duotone" width={32} />
				<span class="text-xl font-bold"
					>Antenne <span class="text-[var(--color-muted-foreground)]">· régie</span></span
				>
			</a>
			<Button variant="ghost" size="sm" onclick={logout}>Déconnexion</Button>
		</header>

		<!-- Prise d'antenne -->
		<Card class="mb-8">
			<div class="flex items-center justify-between">
				<div>
					<h2 class="text-lg font-semibold">Prise d'antenne</h2>
					<p class="text-sm text-[var(--color-muted-foreground)]">
						{live
							? 'Tu es en direct par-dessus la playlist.'
							: 'La playlist tourne. Prends le micro pour passer en direct.'}
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
	</main>
{/if}
