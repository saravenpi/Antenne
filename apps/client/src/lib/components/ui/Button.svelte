<script lang="ts">
	import { cn } from '$lib/utils';
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	type Variant = 'default' | 'outline' | 'ghost' | 'destructive';
	type Size = 'default' | 'sm' | 'lg' | 'icon';

	let {
		variant = 'default',
		size = 'default',
		class: className = '',
		children,
		...rest
	}: {
		variant?: Variant;
		size?: Size;
		class?: string;
		children?: Snippet;
	} & HTMLButtonAttributes = $props();

	const variants: Record<Variant, string> = {
		default: 'bg-[var(--color-primary)] text-[var(--color-primary-foreground)] hover:opacity-90',
		outline: 'border border-[var(--color-border)] bg-transparent hover:bg-[var(--color-muted)]',
		ghost: 'bg-transparent hover:bg-[var(--color-muted)]',
		destructive: 'bg-red-600 text-white hover:bg-red-700'
	};
	const sizes: Record<Size, string> = {
		default: 'h-10 px-4 py-2',
		sm: 'h-8 px-3 text-sm',
		lg: 'h-12 px-6 text-lg',
		icon: 'h-10 w-10'
	};
</script>

<button
	class={cn(
		'inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-[var(--radius)] font-medium transition disabled:pointer-events-none disabled:opacity-50',
		variants[variant],
		sizes[size],
		className
	)}
	{...rest}
>
	{@render children?.()}
</button>
