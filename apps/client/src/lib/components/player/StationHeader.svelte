<script lang="ts">
	import Icon from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import type { Social } from '$lib/api';
	import { socialIcon, socialLabel } from '$lib/socials';

	let { logo, stationName, socials }: { logo: string; stationName: string; socials: Social[] } =
		$props();
</script>

<!-- Station identity, top-left -->
<div class="fixed left-4 top-4 z-40 flex items-center gap-2.5 sm:left-6 sm:top-6">
	<Logo src={logo} size={30} class="shrink-0" />
	<span class="max-w-[45vw] truncate text-xl font-bold tracking-tight sm:max-w-none">
		{stationName}
	</span>
</div>

<!-- Social links, top-right -->
{#if socials.length}
	<div class="fixed right-4 top-4 z-40 flex items-center gap-1.5 sm:right-6 sm:top-6">
		{#each socials as s (s.platform + s.url)}
			<a
				href={s.url}
				target="_blank"
				rel="noopener noreferrer"
				aria-label={socialLabel(s.platform)}
				title={socialLabel(s.platform)}
				class="flex h-8 w-8 items-center justify-center rounded-full text-[var(--color-muted-foreground)] transition hover:bg-white/5 hover:text-[var(--color-foreground)]"
			>
				<Icon icon={socialIcon(s.platform)} width={18} />
			</a>
		{/each}
	</div>
{/if}
