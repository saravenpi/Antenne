<script lang="ts">
	import Icon from '$lib/components/Icon.svelte';
	import { page } from '$app/state';

	const navItems = [
		{ href: '/admin/live', label: "Prise d'antenne", icon: 'lucide:mic' },
		{ href: '/admin/playlist', label: 'Playlist', icon: 'lucide:list-music' },
		{ href: '/admin/clips', label: 'Clips', icon: 'lucide:clapperboard' },
		{ href: '/admin/chat', label: 'Chat & modération', icon: 'lucide:message-circle' },
		{ href: '/admin/appearance', label: 'Apparence', icon: 'lucide:palette' }
	];

	function isActive(href: string): boolean {
		return page.url.pathname === href || page.url.pathname.startsWith(href + '/');
	}
</script>

<nav
	class="fixed inset-x-0 z-50 flex justify-center px-4 md:hidden"
	style="bottom: max(0.75rem, env(safe-area-inset-bottom))"
>
	<div
		class="flex items-center gap-1 rounded-full border border-border/40 bg-background/55 p-1.5 shadow-lg shadow-black/20 ring-1 ring-white/10 backdrop-blur-2xl backdrop-saturate-150"
	>
		{#each navItems as item (item.href)}
			<a
				href={item.href}
				aria-label={item.label}
				title={item.label}
				class="flex items-center justify-center rounded-full px-3 py-2.5 transition-all duration-200 {isActive(
					item.href
				)
					? 'bg-foreground text-background shadow-sm'
					: 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'}"
			>
				<Icon icon={item.icon} width={22} />
			</a>
		{/each}
	</div>
</nav>
