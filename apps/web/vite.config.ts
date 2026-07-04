import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';
import { playwright } from '@vitest/browser-playwright';
import adapter from '@sveltejs/adapter-vercel';
import { sveltekit } from '@sveltejs/kit/vite';

// boringd control plane. Defaults to the public (token-less) endpoint so
// `npm run dev` works with no SSH tunnel; set BORING_URL to a tunnel
// (e.g. http://localhost:18080) to target a local/private boringd. If that
// private boringd needs a token, set BORING_TOKEN and it's injected here
// server-side so it never reaches the browser.
const BORING_URL = process.env.BORING_URL || 'http://localhost:8080';
const BORING_TOKEN = process.env.BORING_TOKEN || '';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			adapter: adapter()
		})
	],
	server: {
		proxy: {
			// Browser -> /boring/* -> boringd (token injected here, HTTP + WS).
			'/boring': {
				target: BORING_URL,
				changeOrigin: true,
				ws: true,
				rewrite: (p) => p.replace(/^\/boring/, ''),
				configure: (proxy) => {
					if (!BORING_TOKEN) return;
					const auth = `Bearer ${BORING_TOKEN}`;
					proxy.on('proxyReq', (r) => r.setHeader('authorization', auth));
					proxy.on('proxyReqWs', (r) => r.setHeader('authorization', auth));
				}
			}
		}
	},
	test: {
		expect: { requireAssertions: true },
		projects: [
			{
				extends: './vite.config.ts',
				test: {
					name: 'client',
					browser: {
						enabled: true,
						provider: playwright(),
						instances: [{ browser: 'chromium', headless: true }]
					},
					include: ['src/**/*.svelte.{test,spec}.{js,ts}'],
					exclude: ['src/lib/server/**']
				}
			},

			{
				extends: './vite.config.ts',
				test: {
					name: 'server',
					environment: 'node',
					include: ['src/**/*.{test,spec}.{js,ts}'],
					exclude: ['src/**/*.svelte.{test,spec}.{js,ts}']
				}
			}
		]
	}
});
