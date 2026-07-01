<script lang="ts">
	import Icon from '$lib/components/Icon.svelte';

	let {
		volume = $bindable(),
		muted,
		needsGesture,
		onprimary
	}: { volume: number; muted: boolean; needsGesture: boolean; onprimary: () => void } = $props();
</script>

<!-- Sound controls, floating liquid glass at bottom-center -->
<div
	class="fixed left-1/2 z-40 flex -translate-x-1/2 items-center gap-3 [bottom:max(1rem,env(safe-area-inset-bottom))] sm:[bottom:max(1.5rem,env(safe-area-inset-bottom))]"
>
	<div
		class="flex h-12 items-center gap-3 rounded-full border border-border/40 bg-background/55 px-4 shadow-lg shadow-black/20 ring-1 ring-white/10 backdrop-blur-2xl backdrop-saturate-150"
	>
		<Icon
			icon={muted || volume === 0 ? 'lucide:volume-off' : 'lucide:volume-2'}
			width={18}
			class="shrink-0 text-[var(--color-muted-foreground)]"
		/>
		<input
			type="range"
			min="0"
			max="1"
			step="0.01"
			bind:value={volume}
			aria-label="Volume"
			class="h-1.5 w-20 cursor-pointer appearance-none rounded-full bg-[var(--color-muted)] accent-[var(--color-foreground)] sm:w-40"
		/>
	</div>
	<button
		onclick={onprimary}
		aria-label={needsGesture ? 'Activer le son' : muted ? 'Réactiver le son' : 'Couper le son'}
		class="flex h-12 w-12 shrink-0 items-center justify-center rounded-full border border-border/40 bg-background/55 text-[var(--color-foreground)] shadow-lg shadow-black/20 ring-1 ring-white/10 backdrop-blur-2xl backdrop-saturate-150 transition hover:bg-background/70"
	>
		<Icon
			icon={needsGesture
				? 'lucide:play'
				: muted
					? 'lucide:volume-off'
					: 'lucide:volume-2'}
			width={22}
		/>
	</button>
</div>
