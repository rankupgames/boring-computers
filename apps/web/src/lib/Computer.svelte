<script lang="ts">
	import '@xterm/xterm/css/xterm.css';
	import { onMount } from 'svelte';
	import { apiBase, wsUrl, createMachine, getMachine, type Machine } from '$lib/boring';

	type Phase = 'idle' | 'booting' | 'live' | 'closed' | 'error';

	let {
		onClose,
		ttl = 60,
		machineId,
		net = false
	}: { onClose?: () => void; ttl?: number; machineId?: string; net?: boolean } = $props();

	let phase = $state<Phase>('idle');
	let machine = $state<Machine | null>(null);
	let error = $state('');
	let remaining = $state(0);
	let shared = $state(false);
	let copied = $state(false);

	// AI command box: an agent that types commands into this shell to reach a goal.
	let agentGoal = $state('');
	let agentRunning = $state(false);
	let agentLine = $state('');
	let agentWs: WebSocket | null = null;

	let host = $state<HTMLDivElement>();
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let term: any = null;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let fit: any = null;
	let ws: WebSocket | null = null;
	let countdown: ReturnType<typeof setInterval> | null = null;
	let onResize: (() => void) | null = null;

	// The component only mounts once the user has asked for a computer, so boot
	// immediately (bind:this on the parent isn't populated until after mount).
	onMount(() => {
		void launch();
	});

	export async function launch() {
		if (phase === 'booting' || phase === 'live') return;
		phase = 'booting';
		error = '';
		try {
			machine = machineId ? await getMachine(machineId) : await createMachine('python', ttl, net);
			await openTerminal(machine!.id);
			phase = 'live';
			startCountdown();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
			phase = 'error';
		}
	}

	async function openTerminal(id: string) {
		const { Terminal } = await import('@xterm/xterm');
		const { FitAddon } = await import('@xterm/addon-fit');
		term = new Terminal({
			fontFamily: "'Geist Mono', ui-monospace, monospace",
			fontSize: 13,
			cursorBlink: true,
			theme: {
				background: '#0a0a0a',
				foreground: '#ededed',
				cursor: '#ededed',
				selectionBackground: '#003674',
				black: '#000000',
				green: '#00ca50',
				brightBlack: '#7d7d7d',
				white: '#ededed'
			}
		});
		fit = new FitAddon();
		term.loadAddon(fit);
		term.open(host!);
		fit.fit();
		onResize = () => fit?.fit();
		window.addEventListener('resize', onResize);

		ws = new WebSocket(wsUrl(`/v1/machines/${id}/tty`));
		ws.binaryType = 'arraybuffer';
		const enc = new TextEncoder();
		ws.onmessage = (e) => {
			if (e.data instanceof ArrayBuffer) term.write(new Uint8Array(e.data));
			else term.write(e.data);
		};
		ws.onclose = () => {
			if (phase === 'live') {
				term?.write('\r\n\x1b[38;5;244m— computer stopped —\x1b[0m\r\n');
				phase = 'closed';
				stopCountdown();
			}
		};
		term.onData((d: string) => ws?.readyState === WebSocket.OPEN && ws.send(enc.encode(d)));
		// Copy the selection with Cmd+C (mac) or Ctrl+Shift+C; a bare Ctrl+C with no
		// selection still passes through as SIGINT. Paste (Cmd+V / Ctrl+Shift+V) is
		// handled natively by xterm — the pasted text arrives via onData above.
		term.attachCustomKeyEventHandler((e: KeyboardEvent) => {
			if (
				e.type === 'keydown' &&
				e.key.toLowerCase() === 'c' &&
				(e.metaKey || (e.ctrlKey && e.shiftKey))
			) {
				const sel = term.getSelection();
				if (sel) {
					void navigator.clipboard?.writeText(sel).catch(() => {});
					return false;
				}
			}
			return true;
		});
		// Clear the boot/restore scrollback to a clean prompt, then nudge the guest.
		setTimeout(() => {
			if (!term) return;
			term.reset();
			term.write(
				`\x1b[38;5;244mboring computers · ephemeral microVM · python3 + node ${net ? '· internet' : 'ready'}\x1b[0m\r\n`
			);
			if (ws?.readyState === WebSocket.OPEN) ws.send(enc.encode('\n'));
		}, 450);
		term.focus();
	}

	function startCountdown() {
		// Prefer the server's expiry (works for both fresh boots and reconnects).
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
		const url = `${location.origin}/c/${machine.id}`;
		try {
			await navigator.clipboard.writeText(url);
		} catch {
			/* ignore */
		}
		shared = true; // keep the machine alive for its TTL even if this tab closes
		copied = true;
		setTimeout(() => (copied = false), 1600);
	}

	function runAI() {
		const goal = agentGoal.trim();
		if (!goal || agentRunning || !machine) return;
		agentRunning = true;
		agentLine = 'thinking…';
		const w = new WebSocket(
			wsUrl(`/v1/machines/${machine.id}/shell-agent?goal=${encodeURIComponent(goal)}`)
		);
		agentWs = w;
		w.onmessage = (e) => {
			let j: { type: string; text: string };
			try {
				j = JSON.parse(e.data);
			} catch {
				return;
			}
			if (j.type === 'done') {
				agentLine = j.text || 'done ✓';
				agentRunning = false;
				w.close();
			} else if (j.type === 'error') {
				agentLine = '⚠ ' + j.text;
				agentRunning = false;
				w.close();
			} else if (j.type === 'say' || j.type === 'action') {
				agentLine = j.text;
			}
		};
		w.onclose = () => {
			agentRunning = false;
		};
	}

	function aiKey(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			e.stopPropagation();
			runAI();
		}
	}

	export function close() {
		stopCountdown();
		try {
			agentWs?.close();
		} catch {
			/* ignore */
		}
		agentWs = null;
		if (onResize) window.removeEventListener('resize', onResize);
		onResize = null;
		try {
			ws?.close();
		} catch {
			/* ignore */
		}
		ws = null;
		// Don't tear down a shared machine, or one we merely reconnected to — let it
		// live its TTL for whoever else has the link.
		if (machine && !shared && !machineId) {
			void fetch(`${apiBase}/v1/machines/${machine.id}`, { method: 'DELETE' }).catch(() => {});
		}
		term?.dispose();
		term = null;
		fit = null;
		machine = null;
		phase = 'idle';
		error = '';
		onClose?.();
	}

	function onKey(e: KeyboardEvent) {
		if (e.key === 'Escape' && phase !== 'idle') close();
	}
</script>

<svelte:window onkeydown={onKey} />

{#if phase !== 'idle'}
	<div class="w-full max-w-3xl">
		<!-- status bar -->
		<div
			class="flex items-center justify-between rounded-t-geist-lg border border-line bg-surface px-4 py-2.5 font-mono text-[12px]"
		>
			<div class="flex items-center gap-2 text-ink-muted">
				{#if phase === 'booting'}
					<span class="size-1.5 animate-pulse rounded-full bg-ink-subtle"></span>booting a computer…
				{:else if phase === 'live' && machine}
					<span class="size-1.5 rounded-full bg-success"></span>
					<span class="text-ink">{machine.id}</span>
					<span class="text-ink-faint">·</span>
					booted in {machine.boot_ms}ms
					<span class="text-ink-faint">·</span>
					{machine.mode}
				{:else if phase === 'closed'}
					<span class="size-1.5 rounded-full bg-ink-faint"></span>computer stopped
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
					<span>self-destructs in {remaining}s</span>
				{/if}
				<button class="text-ink-subtle transition-colors hover:text-ink" onclick={close}>
					esc ✕
				</button>
			</div>
		</div>
		<!-- terminal -->
		<div
			class="border-x border-line bg-[#0a0a0a] p-3"
			class:hidden={phase === 'error'}
			class:rounded-b-geist-lg={phase !== 'live'}
			class:border-b={phase !== 'live'}
		>
			<div bind:this={host} class="h-[420px] w-full"></div>
		</div>

		<!-- AI command box: tell the computer what to do, it drives the terminal -->
		{#if phase === 'live'}
			<div class="rounded-b-geist-lg border border-t-0 border-line bg-surface px-3 py-2.5">
				<div class="flex items-center gap-2">
					<span class="font-mono text-[11px] font-semibold text-accent">AI</span>
					<input
						bind:value={agentGoal}
						onkeydown={aiKey}
						disabled={agentRunning}
						placeholder="tell the computer what to do — e.g. “build a snake game in python and run it”"
						class="min-w-0 flex-1 bg-transparent font-mono text-[12px] text-ink placeholder:text-ink-faint focus:outline-none disabled:opacity-60"
					/>
					<button
						onclick={runAI}
						disabled={agentRunning || !agentGoal.trim()}
						class="rounded-geist bg-ink px-2.5 py-1 font-mono text-[11px] text-black transition-opacity hover:opacity-90 disabled:opacity-40"
					>
						{agentRunning ? 'working…' : 'run'}
					</button>
				</div>
				{#if agentLine}
					<p class="mt-2 flex items-center gap-1.5 font-mono text-[11px] text-ink-muted">
						{#if agentRunning}<span class="size-1.5 animate-pulse rounded-full bg-accent"
							></span>{/if}
						<span class="truncate">{agentLine}</span>
					</p>
				{/if}
			</div>
		{/if}
	</div>
{/if}
