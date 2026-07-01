<script lang="ts">
	import Icon from '$lib/components/Icon.svelte';

	// Renders the station logo: a custom image (URL or data URI) when one is set,
	// otherwise the built-in podcast icon. Keep this the single source of truth for
	// the logo so a régie change reflects everywhere.
	let {
		src = '',
		size = 32,
		class: className = ''
	}: { src?: string; size?: number; class?: string } = $props();
</script>

{#if src}
	<img
		{src}
		alt="Logo"
		width={size}
		height={size}
		class="shrink-0 rounded-md object-contain {className}"
		style="width:{size}px;height:{size}px"
	/>
{:else}
	<Icon icon="solar:podcast-bold-duotone" width={size} class={className} />
{/if}
