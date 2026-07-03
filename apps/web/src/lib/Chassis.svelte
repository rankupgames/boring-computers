<script lang="ts">
	// A retro all-in-one computer case. Whatever renders inside (terminal,
	// desktop, agent) shows up "on the screen". Pure CSS — no image assets.
	import type { Snippet } from 'svelte';

	let { children, on = true }: { children?: Snippet; on?: boolean } = $props();
</script>

<div class="w-full select-none">
	<!-- case -->
	<div
		class="rounded-[26px] bg-[#d9d3c3] p-5 shadow-[inset_0_2px_0_rgba(255,255,255,0.55),inset_0_-8px_16px_rgba(0,0,0,0.14),0_24px_60px_rgba(0,0,0,0.55)]"
	>
		<!-- screen bezel -->
		<div
			class="rounded-[18px] bg-[#c7c1b0] p-4 shadow-[inset_0_3px_8px_rgba(0,0,0,0.28),inset_0_-1px_0_rgba(255,255,255,0.45)] sm:p-6"
		>
			<div
				class="h-[600px] overflow-x-hidden overflow-y-auto rounded-[10px] bg-black p-2 shadow-[inset_0_0_0_3px_#0a0a0a,inset_0_0_24px_rgba(0,0,0,0.9)] [&>div]:max-w-none"
			>
				{@render children?.()}
			</div>
		</div>

		<!-- chin: floppy slit + power LED -->
		<div class="mt-5 flex items-center justify-between px-1">
			<div class="flex items-center gap-2.5">
				<span class="font-mono text-[12px] font-bold tracking-tight text-[#5d5748]">boring.</span>
				<span class="text-[12px] text-[#8a8371]">classic</span>
			</div>
			<div class="flex items-center gap-4">
				<div
					class="h-[7px] w-36 rounded-[3px] bg-[#26241f] shadow-[inset_0_2px_3px_rgba(0,0,0,0.8),0_1px_0_rgba(255,255,255,0.4)] sm:w-48"
				></div>
				<span
					class="size-1.5 rounded-full {on
						? 'bg-success shadow-[0_0_6px_rgba(0,202,80,0.8)]'
						: 'bg-[#a49d8b] shadow-[inset_0_1px_1px_rgba(0,0,0,0.3)]'}"
				></span>
			</div>
		</div>
	</div>

	<!-- pedestal -->
	<div
		class="mx-auto h-5 w-[92%] rounded-b-[16px] bg-[#c4beac] shadow-[inset_0_4px_6px_rgba(0,0,0,0.22),0_16px_32px_rgba(0,0,0,0.45)]"
	></div>
</div>
