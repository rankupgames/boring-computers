<script lang="ts">
	import { onMount } from 'svelte';
	import { apiBase, wsUrl, createMachine, getMachine, type Machine } from '$lib/boring';

	type Phase = 'idle' | 'booting' | 'connecting' | 'live' | 'closed' | 'error';

	let {
		onClose,
		ttl = 180,
		machineId
	}: { onClose?: () => void; ttl?: number; machineId?: string } = $props();

	let phase = $state<Phase>('idle');
	let machine = $state<Machine | null>(null);
	let error = $state('');
	let remaining = $state(0);
	let shared = $state(false);
	let copied = $state(false);

	let screen: HTMLDivElement;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let rfb: any = null;
	let countdown: ReturnType<typeof setInterval> | null = null;
	let attempts = 0;
	let disposed = false;

	const MAX_ATTEMPTS = 10;

	onMount(() => {
		void launch();
		return () => close();
	});

	async function launch() {
		phase = 'booting';
		error = '';
		try {
			machine = machineId ? await getMachine(machineId) : await createMachine('desktop', ttl);
			phase = 'connecting';
			startCountdown();
			// The desktop cold-boots X and paints its apps over a few seconds; wait
			// so noVNC's first full frame has the desktop on it. A reconnect is
			// already painted, so connect quickly.
			setTimeout(connect, machineId ? 300 : 4500);
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
			phase = 'error';
		}
	}

	function teardownRfb() {
		try {
			rfb?.disconnect();
		} catch {
			/* ignore */
		}
		rfb = null;
		// noVNC (not Svelte) injects its canvas into this host node; clear its
		// leftovers so retries don't stack an early black canvas over the live one.
		// eslint-disable-next-line svelte/no-dom-manipulating
		if (screen) screen.innerHTML = '';
	}

	async function connect() {
		if (disposed || !machine) return;
		attempts += 1;
		const { default: RFB } = await import('@novnc/novnc');
		if (disposed) return;
		teardownRfb();
		const url = wsUrl(`/v1/machines/${machine.id}/vnc`);
		try {
			rfb = new RFB(screen, url, {});
			rfb.scaleViewport = true;
			rfb.resizeSession = false;
			rfb.background = '#000';
			rfb.addEventListener('connect', () => {
				if (disposed) return;
				phase = 'live';
			});
			rfb.addEventListener('disconnect', () => {
				if (disposed) return;
				// Early disconnect => x11vnc not ready yet; retry a few times.
				if (phase !== 'live' && attempts < MAX_ATTEMPTS) {
					setTimeout(connect, 1500);
				} else if (phase === 'live') {
					phase = 'closed';
					stopCountdown();
				}
			});
		} catch (e) {
			if (attempts < MAX_ATTEMPTS) setTimeout(connect, 1500);
			else {
				error = e instanceof Error ? e.message : String(e);
				phase = 'error';
			}
		}
	}

	function startCountdown() {
		remaining = machine?.expires_at
			? Math.max(0, Math.round((new Date(machine.expires_at).getTime() - Date.now()) / 1000))
			: ttl;
		countdown = setInterval(() => {
			remaining -= 1;
			if (remaining <= 0) stopCountdown();
		}, 1000);
	}
	function stopCountdown() {
		if (countdown) clearInterval(countdown);
		countdown = null;
	}

	async function copyShare() {
		if (!machine) return;
		try {
			await navigator.clipboard.writeText(`${location.origin}/c/${machine.id}`);
		} catch {
			/* ignore */
		}
		shared = true; // keep the machine alive for its TTL even if this tab closes
		copied = true;
		setTimeout(() => (copied = false), 1600);
	}

	export function close() {
		disposed = true;
		stopCountdown();
		try {
			rfb?.disconnect();
		} catch {
			/* ignore */
		}
		rfb = null;
		if (machine && !shared && !machineId) {
			void fetch(`${apiBase}/v1/machines/${machine.id}`, { method: 'DELETE' }).catch(() => {});
		}
		machine = null;
		phase = 'idle';
		onClose?.();
	}

	function onKey(e: KeyboardEvent) {
		if (e.key === 'Escape') close();
	}
</script>

<svelte:window onkeydown={onKey} />

<div class="w-full max-w-3xl">
	<div
		class="flex items-center justify-between rounded-t-geist-lg border border-line bg-surface px-4 py-2.5 font-mono text-[12px]"
	>
		<div class="flex items-center gap-2 text-ink-muted">
			{#if phase === 'booting'}
				<span class="size-1.5 animate-pulse rounded-full bg-ink-subtle"></span>booting a desktop…
			{:else if phase === 'connecting'}
				<span class="size-1.5 animate-pulse rounded-full bg-ink-subtle"></span>starting the display…
			{:else if phase === 'live' && machine}
				<span class="size-1.5 rounded-full bg-success"></span>
				<span class="text-ink">{machine.id}</span>
				<span class="text-ink-faint">·</span>desktop
				<span class="text-ink-faint">·</span>1280×800
			{:else if phase === 'error'}
				<span class="size-1.5 rounded-full bg-danger"></span>
				<span class="text-danger">{error}</span>
			{/if}
		</div>
		<div class="flex items-center gap-3 text-ink-faint">
			{#if phase === 'live'}
				<button
					class="text-ink-subtle transition-colors hover:text-ink"
					onclick={copyShare}
					title="Copy a link to this computer">{copied ? 'link copied ✓' : 'share'}</button
				>
			{/if}
			{#if phase === 'live' || phase === 'connecting'}<span>self-destructs in {remaining}s</span
				>{/if}
			<button class="text-ink-subtle transition-colors hover:text-ink" onclick={close}>esc ✕</button
			>
		</div>
	</div>
	<div
		class="relative overflow-hidden rounded-b-geist-lg border border-t-0 border-line bg-black"
		class:hidden={phase === 'error'}
	>
		<div bind:this={screen} class="aspect-[16/10] w-full"></div>
		{#if phase !== 'live'}
			<div
				class="pointer-events-none absolute inset-0 flex items-center justify-center font-mono text-[12px] text-ink-subtle"
			>
				{phase === 'booting' ? 'allocating a computer…' : 'painting the screen…'}
			</div>
		{/if}
	</div>
</div>
