import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter(),
		// Content-Security-Policy: SvelteKit hashes its own inline bootstrap
		// scripts automatically. This is the main defence-in-depth control against
		// XSS (and thus token exfiltration from localStorage).
		csp: {
			mode: 'auto',
			directives: {
				'default-src': ['self'],
				'script-src': ['self'],
				'style-src': ['self', 'unsafe-inline'], // Tailwind + inline style attrs (background)
				'img-src': ['self', 'data:', 'blob:'], // covers + data:URL logo/background
				'media-src': ['self', 'blob:'], // MP3/HLS audio (hls.js uses blob: MediaSource)
				'font-src': ['self', 'data:'],
				// Same-origin API/WebSocket + the Iconify icon API (and its fallbacks).
				'connect-src': [
					'self',
					'https://api.iconify.design',
					'https://api.simplesvg.com',
					'https://api.unisvg.com'
				],
				'object-src': ['none'],
				'base-uri': ['self']
			}
		}
	}
};

export default config;
