<script lang="ts">
	import Icon from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import type { Social } from '$lib/api';
	import { socialIcon, socialLabel } from '$lib/socials';

	let { logo, stationName, socials }: { logo: string; stationName: string; socials: Social[] } =
		$props();
</script>

<!-- Station identity, top-left -->
<div class="fixed left-4 top-4 z-40 flex flex-col gap-1.5 sm:left-6 sm:top-6">
	<div class="flex max-w-[55vw] items-center gap-2.5 sm:max-w-none">
		<Logo src={logo} size={30} class="shrink-0" />
		<span class="truncate text-xl font-bold tracking-tight">{stationName}</span>
	</div>
	{#if socials.length}
		<div class="flex items-center gap-2 pl-0.5">
			{#each socials as s (s.platform + s.url)}
				<a
					href={s.url}
					target="_blank"
					rel="noopener noreferrer"
					aria-label={socialLabel(s.platform)}
					title={socialLabel(s.platform)}
					class="flex h-7 w-7 items-center justify-center rounded-full text-[var(--color-muted-foreground)] transition hover:text-[var(--color-foreground)]"
				>
					<Icon icon={socialIcon(s.platform)} width={18} />
				</a>
			{/each}
		</div>
	{/if}
</div>
