<script lang="ts">
	import Computer from '$lib/Computer.svelte';
	import Desktop from '$lib/Desktop.svelte';
	import Agent from '$lib/Agent.svelte';

	type Mode = null | 'shell' | 'desktop' | 'agent';
	let mode = $state<Mode>(null);

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
		<Computer onClose={() => (mode = null)} />
	{:else if mode === 'desktop'}
		<Desktop onClose={() => (mode = null)} />
	{:else if mode === 'agent'}
		<Agent onClose={() => (mode = null)} />
	{:else}
		<div class="flex flex-col items-center gap-3">
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
		</div>
	{/if}
</div>
