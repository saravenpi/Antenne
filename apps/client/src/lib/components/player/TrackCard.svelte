<script lang="ts">
	import Icon from '$lib/components/Icon.svelte';

	// A compact horizontal card for a single track: cover thumbnail + a quick
	// label, title and artist. Used side by side for the on-air / up-next readout.
	let {
		label,
		title,
		artist = '',
		coverUrl = '',
		icon = 'lucide:music',
		accent = false
	}: {
		label: string;
		title: string;
		artist?: string;
		coverUrl?: string;
		icon?: string;
		accent?: boolean;
	} = $props();
</script>

<div
	class="flex min-w-0 flex-1 items-center gap-3 rounded-[var(--radius)] border p-3 transition
		{accent ? 'border-foreground/20 bg-foreground/[0.04]' : 'border-border bg-transparent'}"
>
	<div
		class="relative flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-lg bg-muted"
	>
		{#if coverUrl}
			<img src={coverUrl} alt="" class="h-full w-full object-cover" />
		{:else}
			<Icon icon={icon} width={22} class="text-muted-foreground" />
		{/if}
	</div>
	<div class="min-w-0 flex-1">
		<p class="flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
			{#if accent}
				<span class="h-1.5 w-1.5 shrink-0 animate-pulse rounded-full bg-red-500"></span>
			{/if}
			{label}
		</p>
		<p class="truncate text-sm font-semibold text-foreground">{title || 'Silence radio'}</p>
		{#if artist}
			<p class="truncate text-xs text-muted-foreground">{artist}</p>
		{/if}
	</div>
</div>
