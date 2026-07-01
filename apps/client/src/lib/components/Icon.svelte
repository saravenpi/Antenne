<script lang="ts">
	import Iconify from '@iconify/svelte';

	let {
		icon,
		class: className = '',
		width = 24
	}: { icon: string; class?: string; width?: number | string } = $props();
</script>

<Iconify {icon} class={className} {width} />
