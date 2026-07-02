<script lang="ts">
	import Icon from '$lib/components/Icon.svelte';
	import { api } from '$lib/api';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';

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

	async function logout() {
		// Clears the HttpOnly cookie server-side (JS can't remove it itself).
		await api.logout().catch(() => {});
		goto('/login');
	}
</script>

<aside
	class="sticky top-0 hidden h-[100dvh] w-60 flex-col border-r border-border bg-background md:flex"
>
	<div class="flex items-center gap-2.5 px-5 py-5">
		<Icon icon="solar:podcast-bold-duotone" width={28} />
		<span class="text-2xl font-bold tracking-tight">Antenne</span>
		<span class="text-xs text-muted-foreground">· régie</span>
	</div>

	<nav class="flex flex-1 flex-col gap-1 px-3">
		{#each navItems as item (item.href)}
			<a
				href={item.href}
				class="flex items-center gap-3 rounded-md px-3 py-2.5 text-sm transition-colors {isActive(
					item.href
				)
					? 'bg-foreground font-medium text-background'
					: 'text-muted-foreground hover:bg-muted hover:text-foreground'}"
			>
				<Icon icon={item.icon} width={18} />
				<span>{item.label}</span>
			</a>
		{/each}
	</nav>

	<div class="flex flex-col gap-1 border-t border-border px-3 py-3">
		<a
			href="/"
			target="_blank"
			rel="noopener noreferrer"
			class="flex items-center gap-3 rounded-md px-3 py-2.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
		>
			<Icon icon="lucide:audio-lines" width={18} />
			<span>La radio</span>
		</a>
		<button
			type="button"
			onclick={logout}
			class="flex items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-red-500"
		>
			<Icon icon="lucide:log-out" width={18} />
			<span>Déconnexion</span>
		</button>
	</div>
</aside>
