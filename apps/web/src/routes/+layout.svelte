<script lang="ts">
	import './layout.css';
	import { resolve } from '$app/paths';
	import favicon from '$lib/assets/favicon.svg';

	let { children } = $props();
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<nav
	class="absolute inset-x-0 top-0 z-20 flex items-center justify-between px-5 py-4 font-mono text-[12px]"
>
	<a href={resolve('/')} class="font-semibold text-ink transition-opacity hover:opacity-80"
		>boring.</a
	>
	<a href={resolve('/docs')} class="text-ink-subtle transition-colors hover:text-ink">Docs</a>
</nav>

{@render children()}
