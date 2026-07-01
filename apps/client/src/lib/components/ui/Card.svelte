<script lang="ts">
	import { cn } from '$lib/utils';
	import type { Snippet } from 'svelte';

	let { class: className = '', children }: { class?: string; children?: Snippet } = $props();
</script>

<div
	class={cn(
		'rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-card)] p-6',
		className
	)}
>
	{@render children?.()}
</div>
