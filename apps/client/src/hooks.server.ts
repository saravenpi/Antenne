import type { Handle } from '@sveltejs/kit';

// Security response headers applied to every page/response. The
// Content-Security-Policy itself is configured in svelte.config.js (so SvelteKit
// can hash its inline scripts); these are the complementary headers.
export const handle: Handle = async ({ event, resolve }) => {
	const response = await resolve(event);
	response.headers.set('X-Frame-Options', 'DENY'); // clickjacking
	response.headers.set('X-Content-Type-Options', 'nosniff');
	response.headers.set('Referrer-Policy', 'strict-origin-when-cross-origin');
	// The régie needs the microphone (live broadcast); nothing else.
	response.headers.set('Permissions-Policy', 'geolocation=(), camera=(), microphone=(self)');
	// Honoured only over HTTPS (ignored on plain-HTTP/localhost), safe to always set.
	response.headers.set('Strict-Transport-Security', 'max-age=31536000; includeSubDomains');
	return response;
};
