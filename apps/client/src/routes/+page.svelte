<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import Hls from 'hls.js';
	import Icon from '$lib/components/Icon.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import { api, type NowPlaying } from '$lib/api';

	let audio: HTMLAudioElement;
	let hls: Hls | null = null;
	let playing = $state(false);
	let np = $state<NowPlaying | null>(null);
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

	async function toggle() {
		if (playing) {
			audio.pause();
			playing = false;
			return;
		}
		if (!hls && !audio.src) attach();
		await audio.play();
		playing = true;
	}

	async function refresh() {
		try {
			np = await api.nowPlaying();
		} catch {
			np = null;
		}
	}

	onMount(() => {
		refresh();
		poll = setInterval(refresh, 5000);
	});
	onDestroy(() => {
		clearInterval(poll);
		hls?.destroy();
	});
</script>

<main class="mx-auto flex min-h-screen max-w-md flex-col items-center justify-center gap-10 px-6">
	<audio bind:this={audio} class="hidden"></audio>

	<div class="flex flex-col items-center gap-3">
		<Icon icon="solar:podcast-bold-duotone" width={72} />
		<h1 class="text-4xl font-bold tracking-tight">Antenne</h1>
	</div>

	<div class="flex flex-col items-center gap-1 text-center">
		{#if np?.live}
			<span
				class="flex items-center gap-2 rounded-full border border-red-500/40 px-3 py-1 text-xs font-medium text-red-500"
			>
				<span class="h-2 w-2 animate-pulse rounded-full bg-red-500"></span> EN DIRECT
			</span>
		{/if}
		<p class="mt-2 text-lg font-medium">
			{np?.title || 'Silence radio'}
		</p>
		{#if np?.artist}
			<p class="text-sm text-[var(--color-muted-foreground)]">{np.artist}</p>
		{/if}
	</div>

	<Button size="lg" class="h-20 w-20 rounded-full" onclick={toggle}>
		<Icon icon={playing ? 'solar:pause-bold' : 'solar:play-bold'} width={32} />
	</Button>

	<div class="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
		<Icon icon="solar:users-group-rounded-bold-duotone" width={18} />
		<span>{np?.listeners ?? 0} à l'écoute</span>
	</div>
</main>
