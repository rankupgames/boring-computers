<script lang="ts">
	import { onMount } from 'svelte';
	import Computer from '$lib/Computer.svelte';
	import Desktop from '$lib/Desktop.svelte';
	import Agent from '$lib/Agent.svelte';
	import { fleetCount } from '$lib/boring';

	type Mode = null | 'shell' | 'desktop' | 'agent';
	let mode = $state<Mode>(null);

	let fleet = $state(0);
	onMount(() => {
		const tick = async () => (fleet = await fleetCount());
		void tick();
		const t = setInterval(tick, 4000);
		return () => clearInterval(t);
	});

	// Session length for the shell + desktop (clamped server-side to 15–900s).
	const LENGTHS = [
		{ s: 60, l: '1 min' },
		{ s: 300, l: '5 min' },
		{ s: 900, l: '15 min' }
	];
	let ttl = $state(60);

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && mode === null) {
			const el = document.activeElement;
			if (el && ['INPUT', 'TEXTAREA'].includes(el.tagName)) return;
			if (el?.closest('.xterm')) return;
			mode = 'shell';
		}
	}
</script>

<svelte:head>
	<title>Boring Computers</title>
	<meta name="description" content="Computers that are refreshingly boring." />
</svelte:head>

<svelte:window onkeydown={onKeydown} />

<div class="flex min-h-screen flex-col items-center justify-center gap-8 bg-black px-5 py-16">
	<h1
		class="text-center text-[clamp(1rem,3vw,2rem)] font-semibold whitespace-nowrap tracking-[-0.03em] text-ink"
	>
		Computers that are
		<span class="text-ink-subtle">refreshingly boring.</span>
	</h1>

	{#if mode === 'shell'}
		<Computer {ttl} onClose={() => (mode = null)} />
	{:else if mode === 'desktop'}
		<Desktop {ttl} onClose={() => (mode = null)} />
	{:else if mode === 'agent'}
		<Agent onClose={() => (mode = null)} />
	{:else}
		<div class="flex flex-col items-center gap-4">
			<div class="flex flex-col items-center gap-1.5">
				<button
					onclick={() => (mode = 'shell')}
					class="group inline-flex items-center gap-2 font-mono text-[13px] text-ink-subtle transition-colors hover:text-ink focus-visible:outline-none"
				>
					<kbd
						class="rounded-[5px] border border-line bg-surface px-1.5 py-0.5 text-ink-muted transition-colors group-hover:border-white/25"
						>⏎</kbd
					>
					<span
						>Press <span class="text-ink-muted group-hover:text-ink">enter</span> to get a computer</span
					>
					<span class="ml-0.5 inline-block h-3.5 w-1.5 animate-pulse bg-ink-subtle align-middle"
					></span>
				</button>
				<span class="font-mono text-[11px] text-ink-faint">python3 · node · full Linux</span>
			</div>

			<!-- session length -->
			<div class="flex items-center gap-1 font-mono text-[11px]">
				<span class="mr-1 text-ink-faint">session</span>
				{#each LENGTHS as opt (opt.s)}
					<button
						onclick={() => (ttl = opt.s)}
						class="rounded-full border px-2 py-0.5 transition-colors {ttl === opt.s
							? 'border-white/30 text-ink'
							: 'border-line text-ink-faint hover:text-ink-muted'}"
					>
						{opt.l}
					</button>
				{/each}
			</div>

			<div class="flex items-center gap-4">
				<button
					onclick={() => (mode = 'desktop')}
					class="font-mono text-[12px] text-ink-faint transition-colors hover:text-ink-muted focus-visible:outline-none"
				>
					or spin up a full desktop →
				</button>
				<button
					onclick={() => (mode = 'agent')}
					class="font-mono text-[12px] text-ink-faint transition-colors hover:text-ink-muted focus-visible:outline-none"
				>
					or watch an AI use one →
				</button>
			</div>

			{#if fleet > 0}
				<p class="font-mono text-[11px] text-ink-faint">
					<span class="text-success">●</span>
					{fleet}
					{fleet === 1 ? 'computer' : 'computers'} running right now
				</p>
			{/if}
		</div>
	{/if}
</div>
