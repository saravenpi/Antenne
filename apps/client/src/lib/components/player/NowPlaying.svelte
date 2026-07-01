<script lang="ts">
	import { fade } from 'svelte/transition';
	import type { NowPlaying } from '$lib/api';

	let { np }: { np: NowPlaying | null } = $props();
</script>

<!-- Now playing -->
<div class="flex flex-col items-center gap-1 text-center">
	{#if np?.coverUrl}
		{#key np.coverUrl}
			<img
				src={np.coverUrl}
				alt=""
				transition:fade
				class="mb-3 h-28 w-28 rounded-xl object-cover shadow-lg shadow-black/30 sm:h-36 sm:w-36"
			/>
		{/key}
	{/if}
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
</div>
