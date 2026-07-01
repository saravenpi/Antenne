<script lang="ts">
	import Icon from '$lib/components/Icon.svelte';

	let { count }: { count: number } = $props();
</script>

<!-- Listener count, floating liquid-glass pill: top-right on mobile, bottom-left from sm up -->
<div
	class="fixed right-4 top-4 z-40 flex h-12 items-center gap-2 rounded-full border border-border/40 bg-background/55 px-4 text-sm text-[var(--color-muted-foreground)] shadow-lg shadow-black/20 ring-1 ring-white/10 backdrop-blur-2xl backdrop-saturate-150 sm:right-auto sm:top-auto sm:left-6 sm:[bottom:max(1.5rem,env(safe-area-inset-bottom))]"
>
	<Icon icon="lucide:users" width={18} class="shrink-0" />
	<span class="relative flex h-2 w-2 shrink-0" aria-hidden="true">
		<span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-500 opacity-75"></span>
		<span class="relative inline-flex h-2 w-2 rounded-full bg-green-500"></span>
	</span>
	<span class="whitespace-nowrap">{count} à l'écoute</span>
</div>
