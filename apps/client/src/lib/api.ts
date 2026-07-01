import { browser } from '$app/environment';

const TOKEN_KEY = 'antenne_token';

export function getToken(): string | null {
	return browser ? localStorage.getItem(TOKEN_KEY) : null;
}

export function setToken(token: string) {
	if (browser) localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
	if (browser) localStorage.removeItem(TOKEN_KEY);
}

export function isAuthed(): boolean {
	return !!getToken();
}

// ---- Types ----

export type NowPlaying = {
	live: boolean;
	paused?: boolean;
	title: string;
	artist: string;
	trackId: string;
	coverUrl?: string;
	listeners: number;
	next?: { title: string; artist: string; trackId: string } | null;
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
	const headers = new Headers(init.headers);
	const token = getToken();
	if (token) headers.set('Authorization', `Bearer ${token}`);
	const res = await fetch(`/api${path}`, { ...init, headers });
	if (!res.ok) {
		const body = await res.json().catch(() => ({}));
		throw new Error(body.error ?? `HTTP ${res.status}`);
	}
	return res.status === 204 ? (undefined as T) : res.json();
}

function jsonBody(method: string, data: unknown): RequestInit {
	return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) };
}

/** WebSocket URL for the live chat. Admins pass their token to receive IPs + mod context. */
export function chatWsUrl(): string {
	if (!browser) return '';
	const proto = location.protocol === 'https:' ? 'wss' : 'ws';
	const token = getToken();
	const q = token ? `?token=${encodeURIComponent(token)}` : '';
	return `${proto}://${location.host}/api/chat/ws${q}`;
}

export const api = {
	// ---- Playback / station ----
	nowPlaying: (atMillis?: number) =>
		req<NowPlaying>('/now-playing' + (atMillis ? `?at=${atMillis}` : '')),
	appearance: () => req<Appearance>('/appearance'),
	login: (username: string, password: string) =>
		req<{ token: string; username: string }>('/auth/login', jsonBody('POST', { username, password })),

	// ---- Tracks / playlist ----
	tracks: () => req<Track[]>('/tracks'),
	rescanMetadata: () => req<Track[]>('/tracks/rescan', { method: 'POST' }),
	deleteTrack: (id: string) => req<void>(`/tracks/${id}`, { method: 'DELETE' }),
	reorder: (order: string[]) => req<{ status: string }>('/playlist', jsonBody('PUT', { order })),
	async upload(file: File, title: string, artist: string): Promise<Track> {
		const form = new FormData();
		form.set('file', file);
		form.set('title', title);
		form.set('artist', artist);
		const headers = new Headers();
		const token = getToken();
		if (token) headers.set('Authorization', `Bearer ${token}`);
		const res = await fetch('/api/tracks', { method: 'POST', headers, body: form });
		if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error ?? 'upload failed');
		return res.json();
	},

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
