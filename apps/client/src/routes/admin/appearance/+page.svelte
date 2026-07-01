<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, type Social } from '$lib/api';
	import Icon from '$lib/components/Icon.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Card from '$lib/components/ui/Card.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import { SOCIAL_PLATFORMS, socialIcon } from '$lib/socials';

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

	// ---- Logo ----
	let logo = $state('');
	let logoUrl = $state('');
	let logoBusy = $state(false);
	let logoSaved = $state(false);
	let logoError = $state<string | null>(null);
	let logoTimer: ReturnType<typeof setTimeout> | null = null;

	function onLogoFile(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		logoError = null;
		const reader = new FileReader();
		reader.onload = () => {
			const img = new Image();
			img.onload = () => {
				try {
					const max = 256;
					let { width, height } = img;
					if (width > height && width > max) {
						height = Math.round((height * max) / width);
						width = max;
					} else if (height > max) {
						width = Math.round((width * max) / height);
						height = max;
					}
					const canvas = document.createElement('canvas');
					canvas.width = width;
					canvas.height = height;
					const ctx = canvas.getContext('2d');
					if (!ctx) throw new Error('Canvas indisponible');
					ctx.drawImage(img, 0, 0, width, height);
					logo = canvas.toDataURL('image/png');
				} catch (err) {
					logoError = err instanceof Error ? err.message : 'Image illisible';
				}
			};
			img.onerror = () => (logoError = 'Image illisible');
			img.src = reader.result as string;
		};
		reader.onerror = () => (logoError = 'Lecture du fichier impossible');
		reader.readAsDataURL(file);
	}

	function applyLogoUrl() {
		const u = logoUrl.trim();
		if (u) logo = u;
	}

	async function saveLogo(value: string) {
		logoBusy = true;
		logoError = null;
		logoSaved = false;
		try {
			await api.saveSettings({ logo: value });
			logoSaved = true;
			if (logoTimer) clearTimeout(logoTimer);
			logoTimer = setTimeout(() => (logoSaved = false), 2500);
		} catch (err) {
			logoError = err instanceof Error ? err.message : 'Enregistrement impossible';
		} finally {
			logoBusy = false;
		}
	}

	function resetLogo() {
		logo = '';
		saveLogo('');
	}

	// ---- Socials ----
	let socials = $state<Social[]>([]);
	let socialsBusy = $state(false);
	let socialsSaved = $state(false);
	let socialsError = $state<string | null>(null);
	let socialsTimer: ReturnType<typeof setTimeout> | null = null;

	function addSocial() {
		socials.push({ platform: 'instagram', url: '' });
	}
	function removeSocial(i: number) {
		socials.splice(i, 1);
	}

	async function saveSocials() {
		socialsBusy = true;
		socialsError = null;
		socialsSaved = false;
		try {
			await api.saveSettings({ socials: socials.filter((s) => s.url.trim()) });
			socialsSaved = true;
			if (socialsTimer) clearTimeout(socialsTimer);
			socialsTimer = setTimeout(() => (socialsSaved = false), 2500);
		} catch (err) {
			socialsError = err instanceof Error ? err.message : 'Enregistrement impossible';
		} finally {
			socialsBusy = false;
		}
	}

	onMount(async () => {
		try {
			const s = await api.settings();
			stationName = s.stationName ?? '';
			background = s.background ?? '';
			logo = s.logo ?? '';
			socials = s.socials ?? [];
		} catch (err) {
			apprError = err instanceof Error ? err.message : 'Chargement impossible';
		}
	});

	onDestroy(() => {
		if (nameTimer) clearTimeout(nameTimer);
		if (apprTimer) clearTimeout(apprTimer);
		if (logoTimer) clearTimeout(logoTimer);
		if (socialsTimer) clearTimeout(socialsTimer);
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

	<!-- ================= Logo ================= -->
	<Card class="mt-6">
		<div class="mb-1 flex items-center gap-2">
			<Icon icon="lucide:image" width={18} class="text-foreground" />
			<h2 class="text-base font-semibold text-foreground">Logo</h2>
		</div>
		<p class="mb-4 text-xs text-muted-foreground">
			Le logo affiché sur la page d'écoute. Laisse vide pour utiliser l'icône par défaut.
		</p>

		<div class="grid grid-cols-1 gap-6 md:grid-cols-2">
			<div class="space-y-4">
				<!-- Upload -->
				<div>
					<label for="logo-file" class="mb-1.5 block text-sm font-medium text-foreground">
						Importer une image
					</label>
					<input
						id="logo-file"
						type="file"
						accept="image/*"
						onchange={onLogoFile}
						class="block w-full text-sm text-muted-foreground file:mr-3 file:rounded-[var(--radius)] file:border file:border-border file:bg-transparent file:px-3 file:py-2 file:text-sm file:text-foreground hover:file:bg-muted"
					/>
					<p class="mt-1 text-xs text-muted-foreground">
						Redimensionnée automatiquement (max 256×256).
					</p>
				</div>

				<!-- URL (advanced) -->
				<div>
					<label for="logo-url" class="mb-1.5 block text-sm font-medium text-foreground">
						URL <span class="text-muted-foreground">(avancé)</span>
					</label>
					<div class="flex gap-2">
						<Input id="logo-url" bind:value={logoUrl} placeholder="https://…/logo.png" />
						<Button type="button" variant="outline" size="sm" onclick={applyLogoUrl}>Appliquer</Button>
					</div>
				</div>

				<div class="flex flex-wrap items-center gap-3">
					<Button type="button" onclick={() => saveLogo(logo)} disabled={logoBusy}>
						Enregistrer le logo
					</Button>
					<Button type="button" variant="outline" size="sm" onclick={resetLogo} disabled={logoBusy}>
						Réinitialiser le logo par défaut
					</Button>
					{#if logoSaved}
						<span class="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
							<Icon icon="lucide:circle-check" width={16} /> Enregistré
						</span>
					{/if}
					{#if logoError}
						<span class="text-sm text-red-500">{logoError}</span>
					{/if}
				</div>
			</div>

			<!-- Live preview -->
			<div>
				<span class="mb-1.5 block text-sm font-medium text-foreground">Aperçu</span>
				<div
					class="flex h-40 items-center justify-center rounded-[var(--radius)] border border-border"
					style="background: var(--color-background);"
				>
					<Logo src={logo} size={56} />
				</div>
			</div>
		</div>
	</Card>

	<!-- ================= Socials ================= -->
	<Card class="mt-6">
		<div class="mb-1 flex items-center gap-2">
			<Icon icon="lucide:share-2" width={18} class="text-foreground" />
			<h2 class="text-base font-semibold text-foreground">Réseaux sociaux</h2>
		</div>
		<p class="mb-4 text-xs text-muted-foreground">
			Les liens affichés sur la page d'écoute. Les lignes vides sont ignorées.
		</p>

		<div class="space-y-3">
			{#each socials as social, i (i)}
				<div class="flex flex-col gap-2 sm:flex-row sm:items-center">
					<span class="flex shrink-0 items-center gap-2 text-muted-foreground">
						<Icon icon={socialIcon(social.platform)} width={18} />
					</span>
					<select
						bind:value={socials[i].platform}
						aria-label="Plateforme"
						class="rounded-[var(--radius)] border border-border bg-transparent px-3 py-2 text-sm sm:w-44"
					>
						{#each SOCIAL_PLATFORMS as p (p.id)}
							<option value={p.id}>{p.label}</option>
						{/each}
					</select>
					<Input
						bind:value={socials[i].url}
						placeholder="https://…"
						aria-label="Lien"
						class="min-w-0 flex-1"
					/>
					<button
						type="button"
						onclick={() => removeSocial(i)}
						aria-label="Supprimer"
						class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--radius)] border border-border text-red-500 transition hover:bg-muted"
					>
						<Icon icon="lucide:trash-2" width={16} />
					</button>
				</div>
			{/each}
		</div>

		<div class="mt-4 flex flex-wrap items-center gap-3">
			<Button type="button" variant="outline" size="sm" onclick={addSocial}>
				<Icon icon="lucide:plus" width={16} class="mr-1.5" /> Ajouter un réseau
			</Button>
			<Button type="button" onclick={saveSocials} disabled={socialsBusy}>
				Enregistrer les réseaux
			</Button>
			{#if socialsSaved}
				<span class="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
					<Icon icon="lucide:circle-check" width={16} /> Enregistré
				</span>
			{/if}
			{#if socialsError}
				<span class="text-sm text-red-500">{socialsError}</span>
			{/if}
		</div>
	</Card>
</div>
