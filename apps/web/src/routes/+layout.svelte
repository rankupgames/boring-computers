<script lang="ts">
	import './layout.css';
	import { resolve } from '$app/paths';
	import favicon from '$lib/assets/favicon.svg';

	let { children } = $props();
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<nav
	class="max-w-4xl mx-auto absolute inset-x-0 top-0 z-20 flex items-center justify-between px-5 py-4 font-mono text-[12px]"
>
	<a href={resolve('/')} class="font-semibold text-ink transition-opacity hover:opacity-80 p-2"
		>boring.</a
	>
	<div class="flex items-center gap-5">
		<a href={resolve('/inference')} class="text-ink-subtle transition-colors hover:text-ink"
			>Inference</a
		>
		<a href={resolve('/docs')} class="text-ink-subtle transition-colors hover:text-ink">Docs</a>
	</div>
</nav>

{@render children()}
