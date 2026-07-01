<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import Icon from '$lib/components/Icon.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Input from '$lib/components/ui/Input.svelte';

	// ---- Station name ----
	let stationName = $state('');
	let nameBusy = $state(false);
	let nameSaved = $state(false);
	let nameError = $state<string | null>(null);
	let nameTimer: ReturnType<typeof setTimeout> | null = null;

	async function saveName(e: SubmitEvent) {
		e.preventDefault();
		nameBusy = true;
		nameError = null;
		nameSaved = false;
		try {
			await api.saveSettings({ stationName: stationName.trim() });
			nameSaved = true;
			if (nameTimer) clearTimeout(nameTimer);
			nameTimer = setTimeout(() => (nameSaved = false), 2500);
		} catch (err) {
			nameError = err instanceof Error ? err.message : 'Enregistrement impossible';
		} finally {
			nameBusy = false;
		}
	}

	// ---- Appearance (public listener page background) ----
	let background = $state('');
	let bgColor = $state('#0a0a0a');
	let bgImageUrl = $state('');
	let apprBusy = $state(false);
	let apprSaved = $state(false);
	let apprError = $state<string | null>(null);
	let apprTimer: ReturnType<typeof setTimeout> | null = null;

	const bgPresets: { label: string; value: string }[] = [
		{ label: 'Défaut', value: '' },
		{ label: 'Nuit', value: 'radial-gradient(circle at 50% 0%, #1b1b1b, #0a0a0a)' },
		{ label: 'Violet', value: 'linear-gradient(160deg, #241b4d, #0a0a0a)' },
		{ label: 'Braise', value: 'linear-gradient(160deg, #3b0d0d, #0a0a0a)' },
		{ label: 'Forêt', value: 'linear-gradient(160deg, #0b2b1e, #0a0a0a)' },
		{ label: 'Océan', value: 'linear-gradient(160deg, #0b2540, #0a0a0a)' }
	];

	function applyColor() {
		background = bgColor;
	}
	function applyImage() {
		const u = bgImageUrl.trim();
		if (u) background = `url("${u}") center/cover no-repeat fixed`;
	}

	async function saveAppearance() {
		apprBusy = true;
		apprError = null;
		apprSaved = false;
		try {
			await api.saveSettings({ background });
			apprSaved = true;
			if (apprTimer) clearTimeout(apprTimer);
			apprTimer = setTimeout(() => (apprSaved = false), 2500);
		} catch (err) {
			apprError = err instanceof Error ? err.message : 'Enregistrement impossible';
		} finally {
			apprBusy = false;
		}
	}

	onMount(async () => {
		try {
			const s = await api.settings();
			stationName = s.stationName ?? '';
			background = s.background ?? '';
		} catch (err) {
			apprError = err instanceof Error ? err.message : 'Chargement impossible';
		}
	});

	onDestroy(() => {
		if (nameTimer) clearTimeout(nameTimer);
		if (apprTimer) clearTimeout(apprTimer);
	});
</script>

<div class="mx-auto max-w-4xl px-4 py-6 sm:px-6 sm:py-10 md:px-10">
	<div class="mb-8">
		<h1 class="text-xl font-semibold tracking-tight text-foreground sm:text-2xl">Apparence</h1>
		<p class="mt-1 text-sm text-muted-foreground">
			Personnalise l'identité et l'ambiance de la page d'écoute : nom de la radio et fond.
		</p>
	</div>

	<!-- ================= Station name ================= -->
	<Card>
		<div class="mb-1 flex items-center gap-2">
			<Icon icon="lucide:tag" width={18} class="text-foreground" />
			<h2 class="text-base font-semibold text-foreground">Nom de la radio</h2>
		</div>
		<p class="mb-4 text-xs text-muted-foreground">
			Le nom affiché sur la page d'écoute et dans l'onglet du navigateur.
		</p>

		<form class="flex flex-col gap-3 sm:flex-row sm:items-center" onsubmit={saveName}>
			<Input
				bind:value={stationName}
				placeholder="Antenne"
				aria-label="Nom de la radio"
				class="sm:max-w-sm"
			/>
			<div class="flex items-center gap-3">
				<Button type="submit" disabled={nameBusy || !stationName.trim()}>Enregistrer</Button>
				{#if nameSaved}
					<span class="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
						<Icon icon="lucide:circle-check" width={16} /> Enregistré
					</span>
				{/if}
				{#if nameError}
					<span class="text-sm text-red-500">{nameError}</span>
				{/if}
			</div>
		</form>
	</Card>

	<!-- ================= Appearance ================= -->
	<Card class="mt-6">
		<div class="mb-1 flex items-center gap-2">
			<Icon icon="lucide:palette" width={18} class="text-foreground" />
			<h2 class="text-base font-semibold text-foreground">Apparence de la page auditeur</h2>
		</div>
		<p class="mb-4 text-xs text-muted-foreground">
			Personnalise le fond de la page d'écoute (couleur, dégradé ou image).
		</p>

		<div class="grid grid-cols-1 gap-6 md:grid-cols-2">
			<div class="space-y-4">
				<!-- Presets -->
				<div>
					<span class="mb-1.5 block text-sm font-medium text-foreground">Ambiances</span>
					<div class="flex flex-wrap gap-2">
						{#each bgPresets as p (p.label)}
							<button
								type="button"
								onclick={() => (background = p.value)}
								class="rounded-full border border-border px-3 py-1.5 text-xs transition hover:bg-muted {background ===
								p.value
									? 'bg-foreground text-background'
									: 'text-muted-foreground'}"
							>
								{p.label}
							</button>
						{/each}
					</div>
				</div>

				<!-- Color -->
				<div>
					<span class="mb-1.5 block text-sm font-medium text-foreground">Couleur</span>
					<div class="flex items-center gap-2">
						<input
							type="color"
							bind:value={bgColor}
							aria-label="Couleur de fond"
							class="h-10 w-14 cursor-pointer rounded-[var(--radius)] border border-border bg-transparent"
						/>
						<Button type="button" variant="outline" size="sm" onclick={applyColor}>
							Utiliser cette couleur
						</Button>
					</div>
				</div>

				<!-- Image -->
				<div>
					<label for="bg-image" class="mb-1.5 block text-sm font-medium text-foreground">
						Image (URL)
					</label>
					<div class="flex gap-2">
						<Input id="bg-image" bind:value={bgImageUrl} placeholder="https://…/image.jpg" />
						<Button type="button" variant="outline" size="sm" onclick={applyImage}>Appliquer</Button>
					</div>
				</div>

				<!-- Raw CSS (advanced) -->
				<div>
					<label for="bg-raw" class="mb-1.5 block text-sm font-medium text-foreground">
						Valeur CSS <span class="text-muted-foreground">(avancé)</span>
					</label>
					<Input id="bg-raw" bind:value={background} placeholder="vide = thème par défaut" />
				</div>

				<div class="flex items-center gap-3">
					<Button type="button" onclick={saveAppearance} disabled={apprBusy}>Enregistrer le fond</Button>
					<Button type="button" variant="ghost" size="sm" onclick={() => (background = '')}>
						Réinitialiser
					</Button>
					{#if apprSaved}
						<span class="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
							<Icon icon="lucide:circle-check" width={16} /> Enregistré
						</span>
					{/if}
					{#if apprError}
						<span class="text-sm text-red-500">{apprError}</span>
					{/if}
				</div>
			</div>

			<!-- Live preview -->
			<div>
				<span class="mb-1.5 block text-sm font-medium text-foreground">Aperçu</span>
				<div
					class="flex h-56 items-center justify-center rounded-[var(--radius)] border border-border"
					style={background ? `background: ${background};` : 'background: var(--color-background);'}
				>
					<div class="flex flex-col items-center gap-2 text-foreground">
						<Icon icon="solar:podcast-bold-duotone" width={40} />
						<span class="text-lg font-bold">{stationName || 'Antenne'}</span>
					</div>
				</div>
			</div>
		</div>
	</Card>
</div>
