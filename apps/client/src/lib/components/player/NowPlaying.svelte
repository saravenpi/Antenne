<script lang="ts">
	import { fade } from 'svelte/transition';
	import Icon from '$lib/components/Icon.svelte';
	import type { NowPlaying } from '$lib/api';

	let { np }: { np: NowPlaying | null } = $props();
</script>

<!-- Compact now-playing card: cover on the left, title/artist on the right -->
<div
	class="flex w-full max-w-sm items-center gap-3 rounded-[var(--radius)] border border-border bg-white/[0.03] p-3"
>
	<div
		class="relative flex h-16 w-16 shrink-0 items-center justify-center overflow-hidden rounded-lg bg-[var(--color-muted)]"
	>
		{#if np?.coverUrl}
			{#key np.coverUrl}
				<img src={np.coverUrl} alt="" transition:fade class="h-full w-full object-cover" />
			{/key}
		{:else}
			<Icon icon="lucide:radio" width={24} class="text-[var(--color-muted-foreground)]" />
		{/if}
	</div>

	<div class="min-w-0 flex-1 text-left">
		{#if np?.live}
			<span class="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-red-500">
				<span class="h-1.5 w-1.5 animate-pulse rounded-full bg-red-500"></span> En direct
			</span>
		{/if}
		<p class="truncate text-sm font-semibold text-[var(--color-foreground)]">
			{np?.title || 'Silence radio'}
		</p>
		{#if np?.artist}
			<p class="truncate text-xs text-[var(--color-muted-foreground)]">{np.artist}</p>
		{/if}
	</div>
</div>
