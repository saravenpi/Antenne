<script lang="ts">
	import { cn } from '$lib/utils';
	import type { HTMLInputAttributes } from 'svelte/elements';

	let {
		class: className = '',
		value = $bindable(),
		...rest
	}: { class?: string; value?: string } & HTMLInputAttributes = $props();
</script>

<input
	bind:value
	class={cn(
		'h-10 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-transparent px-3 text-sm outline-none placeholder:text-[var(--color-muted-foreground)] focus:ring-2 focus:ring-[var(--color-foreground)]/20',
		className
	)}
	{...rest}
/>
