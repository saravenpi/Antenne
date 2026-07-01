<script lang="ts">
	import { onMount } from 'svelte';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import BottomNav from '$lib/components/BottomNav.svelte';
	import { getToken } from '$lib/api';
	import { goto } from '$app/navigation';

	let { children } = $props();

	let ready = $state(false);

	onMount(() => {
		if (!getToken()) {
			goto('/login');
			return;
		}
		ready = true;
	});
</script>

{#if ready}
	<div class="flex h-[100dvh] w-full overflow-hidden">
		<Sidebar />
		<main class="flex-1 overflow-auto pb-24 md:pb-0">
			{@render children()}
		</main>
		<BottomNav />
	</div>
{/if}
