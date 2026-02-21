<script lang="ts">
	import type { Finding } from '$lib/stores/agent.svelte';
	import { slide } from 'svelte/transition';

	let { finding } = $props<{ finding: Finding }>();
	let isExpanded = $state(false);

	function severityColor(severity: string): string {
		switch (severity?.toLowerCase()) {
			case 'critical': return 'bg-red-500/15 text-red-400 border-red-500/30';
			case 'high': return 'bg-orange-500/15 text-orange-400 border-orange-500/30';
			case 'medium': return 'bg-yellow-500/15 text-yellow-400 border-yellow-500/30';
			case 'low': return 'bg-blue-500/15 text-blue-400 border-blue-500/30';
			default: return 'bg-neutral-500/15 text-neutral-400 border-neutral-500/30';
		}
	}
</script>

<button
	class="w-full text-left bg-neutral-900 border border-neutral-800 rounded-lg p-4 hover:border-neutral-700 transition-colors focus:outline-none"
	onclick={() => isExpanded = !isExpanded}
>
	<div class="flex items-start justify-between">
		<div class="flex items-start gap-3">
			<span class="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-bold uppercase tracking-wider border shrink-0 mt-0.5 {severityColor(finding.severity)} {finding.severity.toLowerCase() === 'critical' ? 'glitch-badge' : ''}">
				{finding.severity}
			</span>
			<div>
				<h3 class="text-sm font-semibold text-neutral-100 leading-snug">
					{finding.title}
				</h3>
				{#if !isExpanded && finding.file_path}
					<div class="text-[11px] text-neutral-500 font-mono mt-1 w-full truncate">
						{finding.file_path}
					</div>
				{/if}
			</div>
		</div>
		<div class="text-neutral-500 shrink-0 ml-4 transition-transform duration-200 {isExpanded ? 'rotate-180' : ''}">
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
			</svg>
		</div>
	</div>

	<!-- Expanded Content -->
	{#if isExpanded}
		<div transition:slide={{ duration: 200 }} class="mt-4 pt-4 border-t border-neutral-800">
			<!-- File path in expanded mode -->
			{#if finding.file_path}
				<div class="text-[11px] text-neutral-400 font-mono mb-3 bg-neutral-950 px-2 py-1 rounded inline-block">
					<span class="text-neutral-500 mr-1">File:</span><span class="text-neutral-300">{finding.file_path}</span>
				</div>
			{/if}

			{#if finding.description}
				<p class="text-sm text-neutral-300 leading-relaxed mb-4">
					{finding.description}
				</p>
			{/if}

			{#if finding.evidence}
				<pre class="bg-black/50 border border-neutral-800 rounded-md p-3 overflow-x-auto text-[11px] font-mono text-neutral-400 whitespace-pre-wrap">{finding.evidence}</pre>
			{/if}
		</div>
	{/if}
</button>

<style>
	@keyframes -global-glitch {
		0% { transform: translate(0) }
		20% { transform: translate(-2px, 1px) }
		40% { transform: translate(-1px, -1px) }
		60% { transform: translate(2px, 1px) }
		80% { transform: translate(1px, -1px) }
		100% { transform: translate(0) }
	}
	
	:global(.glitch-badge) {
		animation: -global-glitch 3s infinite;
		animation-timing-function: steps(2, end);
	}
	/* Add occasional bursts of glitching */
	:global(.glitch-badge:hover) {
		animation: -global-glitch 0.2s infinite;
	}
</style>
