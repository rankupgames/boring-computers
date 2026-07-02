<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { getMachine } from '$lib/boring';

	const home = resolve('/');
	import Computer from '$lib/Computer.svelte';
	import Desktop from '$lib/Desktop.svelte';

	const id = page.params.id ?? '';
	let status = $state<'loading' | 'shell' | 'desktop' | 'gone'>('loading');

	onMount(async () => {
		try {
			const m = await getMachine(id);
			status = m.display ? 'desktop' : 'shell';
		} catch {
			status = 'gone';
		}
	});
</script>

<svelte:head>
	<title>a shared computer · boring computers</title>
</svelte:head>

<div class="flex min-h-screen flex-col items-center justify-center gap-6 bg-black px-5 py-16">
	<a
		href={home}
		class="text-center text-[clamp(1rem,3vw,2rem)] font-semibold whitespace-nowrap tracking-[-0.03em] text-ink"
	>
		Computers that are <span class="text-ink-subtle">refreshingly boring.</span>
	</a>

	{#if status === 'loading'}
		<p class="font-mono text-[12px] text-ink-faint">connecting to a shared computer…</p>
	{:else if status === 'shell'}
		<Computer machineId={id} onClose={() => goto(home)} />
	{:else if status === 'desktop'}
		<Desktop machineId={id} onClose={() => goto(home)} />
	{:else}
		<div class="flex flex-col items-center gap-3">
			<p class="font-mono text-[12px] text-ink-muted">this computer has expired</p>
			<a href={home} class="font-mono text-[12px] text-ink-subtle transition-colors hover:text-ink"
				>← get your own →</a
			>
		</div>
	{/if}
</div>
