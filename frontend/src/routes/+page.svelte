<script lang="ts">
	import * as agent from '$lib/stores/agent.svelte';
	import FindingCard from './FindingCard.svelte';

	// ── Reactive State ──
	let targetPath = $state('');
	let launching = $state(false);

	// ── Derived from store ──
	let status = $derived(agent.getStatus());
	let currentAction = $derived(agent.getCurrentAction());
	let findings = $derived(agent.getFindings());
	let scanSummary = $derived(agent.getScanSummary());
	let storeError = $derived(agent.getError());
	let hackerThought = $derived(agent.getHackerThought());
	let barrelRoll = $derived(agent.getBarrelRoll());

	let displayedSummary = $state('');
	
	$effect(() => {
		if (status === 'completed' && scanSummary?.summary) {
			let i = 0;
			displayedSummary = '';
			const interval = setInterval(() => {
				if (i < scanSummary.summary.length) {
					displayedSummary += scanSummary.summary.charAt(i);
					i++;
				} else {
					clearInterval(interval);
				}
			}, 15);
			return () => clearInterval(interval);
		} else {
			displayedSummary = '';
		}
	});

	// ── Actions ──
	async function handleStartScan() {
		if (!targetPath.trim()) return;
		launching = true;
		await agent.startScan(targetPath.trim());
		launching = false;
	}

	function handleReset() {
		agent.reset();
		targetPath = '';
	}

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

<div class="min-h-screen bg-neutral-950 text-neutral-200 flex flex-col {barrelRoll ? 'animate-roll' : ''}">
	<div class="max-w-3xl w-full mx-auto px-6 pt-24 pb-16 flex-1 flex flex-col">

		<!-- ═══════ STATE 1: IDLE ═══════ -->
		{#if status === 'idle'}
			<div class="flex-1 flex flex-col items-center justify-center text-center">
				<div class="mb-12">
					<h1 class="text-4xl font-semibold tracking-tight text-neutral-100 mb-4">
						Scan your code for security issues
					</h1>
					<p class="text-base text-neutral-500 max-w-lg mx-auto leading-relaxed">
						Argus analyzes your repositories locally for vulnerabilities,
						misconfigurations, and hardcoded secrets.
					</p>
				</div>

				<form
					onsubmit={(e) => { e.preventDefault(); handleStartScan(); }}
					class="w-full max-w-xl space-y-4"
				>
					<input
						type="text"
						bind:value={targetPath}
						placeholder="/home/user/project"
						class="w-full bg-neutral-900 border border-neutral-800 rounded-xl px-5 py-4 text-base text-neutral-200 placeholder-neutral-700 font-mono focus:outline-none focus:border-emerald-700 focus:ring-1 focus:ring-emerald-900/50 transition-all"
					/>
					<button
						type="submit"
						disabled={!targetPath.trim() || launching}
						class="w-full bg-emerald-600 hover:bg-emerald-500 disabled:opacity-30 disabled:cursor-not-allowed text-white font-semibold py-3.5 rounded-xl text-sm tracking-wide transition-all duration-200 cursor-pointer"
					>
						{#if launching}
							Starting...
						{:else}
							Start a scan
						{/if}
					</button>
				</form>

				{#if storeError}
					<div class="mt-6 px-4 py-3 bg-red-950/30 border border-red-900/40 rounded-lg text-red-400 text-sm font-mono max-w-xl w-full text-left">
						{storeError}
					</div>
				{/if}
			</div>

		<!-- ═══════ STATE 2: SCANNING ═══════ -->
		{:else if status === 'scanning'}
			<div class="flex-1 flex flex-col items-center justify-center text-center">
				<!-- Spinner -->
				<div class="w-12 h-12 border-4 border-neutral-800 border-t-emerald-500 rounded-full animate-spin mb-8"></div>

				<h2 class="text-lg font-semibold text-neutral-200 mb-2">
					Scan in progress
				</h2>
				<p class="text-sm text-neutral-600 font-mono animate-pulse">
					{currentAction || 'Initializing...'}
				</p>

				{#if hackerThought}
					<div class="font-mono text-xs text-emerald-500/40 opacity-50 overflow-hidden whitespace-nowrap text-ellipsis max-w-md mx-auto mt-2">
						{hackerThought}
					</div>
				{/if}

				{#if findings.length > 0}
					<div class="mt-8 px-4 py-2 bg-emerald-950/20 border border-emerald-900/30 rounded-lg">
						<span class="text-xs text-emerald-500 font-semibold">
							{findings.length} finding{findings.length !== 1 ? 's' : ''} so far
						</span>
					</div>
				{/if}

				{#if storeError}
					<div class="mt-6 px-4 py-3 bg-red-950/30 border border-red-900/40 rounded-lg text-red-400 text-sm font-mono max-w-xl w-full text-left">
						{storeError}
					</div>
				{/if}
			</div>

		<!-- ═══════ STATE 3: COMPLETED ═══════ -->
		{:else if status === 'completed'}
			<div class="mb-10">
				<h2 class="text-2xl font-semibold text-neutral-100 mb-1">
					Scan Complete
				</h2>
				<p class="text-sm text-neutral-500">
					{findings.length} finding{findings.length !== 1 ? 's' : ''} detected
				</p>
			</div>

			{#if storeError}
				<div class="mb-6 px-4 py-3 bg-red-950/30 border border-red-900/40 rounded-lg text-red-400 text-sm font-mono">
					{storeError}
				</div>
			{/if}

			{#if scanSummary}
				<div class="mb-10 p-6 bg-neutral-900 border border-neutral-800 rounded-xl relative overflow-hidden">
					<div class="absolute inset-0 bg-gradient-to-br from-emerald-500/5 to-transparent pointer-events-none"></div>
					<h3 class="text-lg font-semibold text-neutral-100 mb-4 flex items-center gap-2">
						<svg class="w-5 h-5 text-emerald-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
						</svg>
						Executive Summary
					</h3>
					
					<div class="mb-4">
						<span class="inline-flex items-center px-2.5 py-1 rounded-md text-xs font-bold uppercase tracking-wider border {severityColor(scanSummary.overall_risk)}">
							Risk: {scanSummary.overall_risk}
						</span>
					</div>

					<p class="text-sm text-neutral-300 leading-relaxed mb-6 font-mono">
						{displayedSummary}
					</p>

					{#if scanSummary.attack_chain && displayedSummary.length === scanSummary.summary.length}
						<div class="mt-4">
							<h4 class="text-xs font-semibold text-neutral-400 uppercase tracking-wider mb-3">Hypothetical Attack Chain</h4>
							<div class="text-sm text-neutral-400 border-l-2 border-neutral-700 pl-4 space-y-2 py-1 leading-relaxed whitespace-pre-line">
								{scanSummary.attack_chain}
							</div>
						</div>
					{/if}
				</div>
			{/if}

			{#if findings.length === 0}
				<div class="flex-1 flex flex-col items-center justify-center text-center py-20">
					<div class="w-16 h-16 rounded-full bg-emerald-950/30 border border-emerald-900/30 flex items-center justify-center mb-4">
						<span class="text-2xl">✓</span>
					</div>
					<h3 class="text-lg font-semibold text-neutral-300 mb-2">No issues found</h3>
					<p class="text-sm text-neutral-600">Your codebase looks clean.</p>
				</div>
			{:else}
				<div class="space-y-4 mb-10">
					{#each findings as finding, i}
						<FindingCard {finding} />
					{/each}
				</div>
			{/if}

			<button
				onclick={handleReset}
				class="w-full max-w-xs mx-auto block bg-neutral-900 border border-neutral-800 hover:border-neutral-700 text-neutral-300 font-semibold py-3 rounded-xl text-sm tracking-wide transition-all duration-200 cursor-pointer"
			>
				Start New Scan
			</button>
		{/if}

	</div>
</div>

<style>
	@keyframes roll {
		from { transform: rotate(0deg); }
		to { transform: rotate(360deg); }
	}
	:global(.animate-roll) {
		animation: roll 2s ease-in-out;
	}
</style>
