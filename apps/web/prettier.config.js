import config from '@boring/prettier-config';

/**
 * Shared options live in @boring/prettier-config (packages/prettier-config).
 * tailwindStylesheet is app-specific because the path is relative to this app.
 * @type {import("prettier").Config}
 */
export default {
	...config,
	tailwindStylesheet: './src/routes/layout.css'
};
