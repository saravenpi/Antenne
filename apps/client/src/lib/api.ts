import { browser } from '$app/environment';

// Authentication is carried by an HttpOnly cookie that the API sets on login.
// The token is deliberately NOT accessible to JavaScript (this mitigates XSS
// token theft) and never appears in a URL. Login state is discovered at runtime
// via `api.me()`, and cleared server-side via `api.logout()`.

// ---- Types ----

export type NowPlaying = {
	live: boolean;
	paused?: boolean;
	title: string;
	artist: string;
	trackId: string;
	coverUrl?: string;
	listeners: number;
	next?: { title: string; artist: string; trackId: string; coverUrl?: string } | null;
};

export type Social = { platform: string; url: string };

export type Track = {
	id: string;
	title: string;
	artist: string;
	durationSec: number;
	position: number;
	createdAt: string;
	coverUrl?: string;
	collectionId?: string;
};

export type Collection = {
	id: string;
	name: string;
	position: number;
	active: boolean;
	createdAt: string;
};

export type Clip = {
	id: string;
	title: string;
	durationSec: number;
	createdAt: string;
	url: string; // public audio URL, e.g. /api/clips/<id>/audio
};

export type ChatMessage = {
	id: string;
	name: string;
	body: string;
	createdAt: string;
	ip?: string; // only present for admin sockets/requests
};

export type Ban = {
	id: string;
	ip: string;
	reason: string;
	createdAt: string;
};

export type Restriction = {
	id: string;
	ip: string;
	reason: string;
	createdAt: string;
};

export type Settings = {
	stationName: string;
	bannedWords: string[];
	slowModeSec: number;
	background: string;
	logo: string;
	socials: Social[];
};

export type Appearance = {
	stationName: string;
	background: string;
	logo: string;
	socials: Social[];
};

// Server -> client events on the chat WebSocket.
export type ChatEvent =
	| { type: 'message'; message: ChatMessage }
	| { type: 'delete'; id: string }
	| { type: 'listeners'; count: number }
	| { type: 'error'; error: string };

async function req<T>(path: string, init: RequestInit = {}): Promise<T> {
	// same-origin sends the HttpOnly auth cookie; it is never sent cross-origin.
	const res = await fetch(`/api${path}`, { credentials: 'same-origin', ...init });
	if (!res.ok) {
		const body = await res.json().catch(() => ({}));
		throw new Error(body.error ?? `HTTP ${res.status}`);
	}
	return res.status === 204 ? (undefined as T) : res.json();
}

function jsonBody(method: string, data: unknown): RequestInit {
	return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) };
}

/**
 * WebSocket URL for the live chat. The HttpOnly auth cookie is sent
 * automatically on the same-origin handshake, so admins are recognised without
 * any token in the URL; anonymous visitors simply have no cookie.
 */
export function chatWsUrl(): string {
	if (!browser) return '';
	const proto = location.protocol === 'https:' ? 'wss' : 'ws';
	return `${proto}://${location.host}/api/chat/ws`;
}

export const api = {
	// ---- Playback / station ----
	nowPlaying: (atMillis?: number) =>
		req<NowPlaying>('/now-playing' + (atMillis ? `?at=${atMillis}` : '')),
	appearance: () => req<Appearance>('/appearance'),
	login: (username: string, password: string) =>
		req<{ username: string }>('/auth/login', jsonBody('POST', { username, password })),
	me: () => req<{ username: string }>('/auth/me'),
	logout: () => req<void>('/auth/logout', { method: 'POST' }),

	// ---- Tracks / playlist ----
	tracks: () => req<Track[]>('/tracks'),
	rescanMetadata: () => req<Track[]>('/tracks/rescan', { method: 'POST' }),
	updateTrack: (id: string, data: { title?: string; artist?: string }) =>
		req<Track>(`/tracks/${id}`, jsonBody('PATCH', data)),
	async uploadCover(id: string, file: File): Promise<Track> {
		const form = new FormData();
		form.set('file', file);
		const res = await fetch(`/api/tracks/${id}/cover`, {
			method: 'POST',
			credentials: 'same-origin',
			body: form
		});
		if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error ?? 'upload failed');
		return res.json();
	},
	deleteTrack: (id: string) => req<void>(`/tracks/${id}`, { method: 'DELETE' }),
	reorder: (order: string[]) => req<{ status: string }>('/playlist', jsonBody('PUT', { order })),
	async upload(file: File, title: string, artist: string, collectionId?: string): Promise<Track> {
		const form = new FormData();
		form.set('file', file);
		form.set('title', title);
		form.set('artist', artist);
		if (collectionId) form.set('collectionId', collectionId);
		const res = await fetch('/api/tracks', {
			method: 'POST',
			credentials: 'same-origin',
			body: form
		});
		if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error ?? 'upload failed');
		return res.json();
	},

	// ---- Collections (named playlists / folders) ----
	collections: () => req<Collection[]>('/collections'),
	createCollection: (name: string) => req<Collection>('/collections', jsonBody('POST', { name })),
	renameCollection: (id: string, name: string) =>
		req<void>(`/collections/${id}`, jsonBody('PUT', { name })),
	deleteCollection: (id: string) => req<void>(`/collections/${id}`, { method: 'DELETE' }),
	activateCollection: (id: string) =>
		req<NowPlaying>(`/collections/${id}/activate`, { method: 'POST' }),
	clearActiveCollection: () => req<NowPlaying>('/collections/clear', { method: 'POST' }),
	moveTrack: (id: string, collectionId: string | null) =>
		req<void>(`/tracks/${id}/collection`, jsonBody('PUT', { collectionId })),

	// ---- Live mic ----
	stopLive: () => req<{ status: string }>('/live/stop', { method: 'POST' }),

	// ---- Playback control (manual DJ overrides) ----
	playNext: () => req<NowPlaying>('/playback/next', { method: 'POST' }),
	playPrevious: () => req<NowPlaying>('/playback/previous', { method: 'POST' }),
	pausePlayback: () => req<NowPlaying>('/playback/pause', { method: 'POST' }),
	resumePlayback: () => req<NowPlaying>('/playback/resume', { method: 'POST' }),
	playTrack: (id: string) => req<NowPlaying>(`/tracks/${id}/play`, { method: 'POST' }),

	// ---- Clips ----
	clips: () => req<Clip[]>('/clips'),
	createClip: (seconds: number, title?: string) => req<Clip>('/clips', jsonBody('POST', { seconds, title })),
	deleteClip: (id: string) => req<void>(`/clips/${id}`, { method: 'DELETE' }),

	// ---- Chat ----
	chatHistory: () => req<ChatMessage[]>('/chat/messages'),
	deleteMessage: (id: string) => req<void>(`/chat/messages/${id}`, { method: 'DELETE' }),

	// ---- Moderation ----
	bans: () => req<Ban[]>('/chat/bans'),
	ban: (ip: string, reason?: string) => req<Ban>('/chat/bans', jsonBody('POST', { ip, reason })),
	unban: (id: string) => req<void>(`/chat/bans/${id}`, { method: 'DELETE' }),
	restrictions: () => req<Restriction[]>('/chat/restrictions'),
	restrict: (ip: string, reason?: string) => req<Restriction>('/chat/restrictions', jsonBody('POST', { ip, reason })),
	unrestrict: (id: string) => req<void>(`/chat/restrictions/${id}`, { method: 'DELETE' }),

	// ---- Settings ----
	settings: () => req<Settings>('/settings'),
	saveSettings: (s: Partial<Settings>) => req<Settings>('/settings', jsonBody('PUT', s))
};
