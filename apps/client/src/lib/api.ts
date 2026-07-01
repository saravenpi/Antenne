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

export type NowPlaying = {
	live: boolean;
	title: string;
	artist: string;
	trackId: string;
	listeners: number;
};

export type Track = {
	id: string;
	title: string;
	artist: string;
	durationSec: number;
	position: number;
	createdAt: string;
};

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

export const api = {
	nowPlaying: () => req<NowPlaying>('/now-playing'),
	login: (username: string, password: string) =>
		req<{ token: string; username: string }>('/auth/login', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ username, password })
		}),
	tracks: () => req<Track[]>('/tracks'),
	deleteTrack: (id: string) => req<void>(`/tracks/${id}`, { method: 'DELETE' }),
	reorder: (order: string[]) =>
		req<{ status: string }>('/playlist', {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ order })
		}),
	stopLive: () => req<{ status: string }>('/live/stop', { method: 'POST' }),
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
	}
};
