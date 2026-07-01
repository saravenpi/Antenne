<script lang="ts">
	import { onMount } from 'svelte';
	import Icon from '$lib/components/Icon.svelte';
	import { api, getToken, setToken } from '$lib/api';
	import { goto } from '$app/navigation';

	let username = $state('');
	let password = $state('');
	let error = $state('');
	let busy = $state(false);

	const year = new Date().getFullYear();

	async function login(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = '';
		try {
			const r = await api.login(username, password);
			setToken(r.token);
			goto('/admin/live');
		} catch (err) {
			error = (err as Error).message;
		} finally {
			busy = false;
		}
	}

	onMount(() => {
		if (getToken()) goto('/admin/live');
	});
</script>

<svelte:head>
	<title>Régie — Antenne</title>
</svelte:head>

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
						class="h-10 w-full rounded-md border border-border bg-background px-3 text-sm outline-none placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-foreground/20"
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
						class="h-10 w-full rounded-md border border-border bg-background px-3 text-sm outline-none placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-foreground/20"
					/>
				</div>

				{#if error}
					<p class="text-sm text-red-500">{error}</p>
				{/if}

				<button
					type="submit"
					disabled={busy}
					class="inline-flex h-10 w-full items-center justify-center gap-2 rounded-md bg-primary text-sm font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
				>
					<Icon icon="lucide:log-in" width={18} />
					{busy ? 'Connexion…' : 'Entrer en régie'}
				</button>
			</form>
		</div>
	</div>
</div>
