// Shared list of supported social platforms and their Iconify icons. Brand
// glyphs come from the `simple-icons` set (Lucide dropped most brand logos);
// the generic "website" uses lucide:globe.
export type SocialPlatform = { id: string; label: string; icon: string };

export const SOCIAL_PLATFORMS: SocialPlatform[] = [
	{ id: 'instagram', label: 'Instagram', icon: 'simple-icons:instagram' },
	{ id: 'x', label: 'X / Twitter', icon: 'simple-icons:x' },
	{ id: 'youtube', label: 'YouTube', icon: 'simple-icons:youtube' },
	{ id: 'tiktok', label: 'TikTok', icon: 'simple-icons:tiktok' },
	{ id: 'facebook', label: 'Facebook', icon: 'simple-icons:facebook' },
	{ id: 'twitch', label: 'Twitch', icon: 'simple-icons:twitch' },
	{ id: 'spotify', label: 'Spotify', icon: 'simple-icons:spotify' },
	{ id: 'soundcloud', label: 'SoundCloud', icon: 'simple-icons:soundcloud' },
	{ id: 'discord', label: 'Discord', icon: 'simple-icons:discord' },
	{ id: 'bandcamp', label: 'Bandcamp', icon: 'simple-icons:bandcamp' },
	{ id: 'website', label: 'Site web', icon: 'lucide:globe' }
];

export function socialIcon(platform: string): string {
	return SOCIAL_PLATFORMS.find((p) => p.id === platform)?.icon ?? 'lucide:globe';
}

export function socialLabel(platform: string): string {
	return SOCIAL_PLATFORMS.find((p) => p.id === platform)?.label ?? platform;
}
