/**
 * Argus Scanner State — Minimalist 3-State Store
 *
 * Svelte 5 runes module that tracks:
 * - Scan lifecycle: idle → scanning → completed
 * - Current agent action (for live status display)
 * - Findings reported by the agent
 */

const API_BASE = 'http://localhost:8080';

// ── Types ──

export interface Finding {
	title: string;
	severity: string;
	file_path: string;
	description: string;
	evidence: string;
}

export interface ScanSummary {
	overall_risk: string;
	summary: string;
	attack_chain: string;
}

// ── Reactive State ──

let status = $state<'idle' | 'scanning' | 'completed'>('idle');
let currentAction = $state<string>('Initializing scan...');
let findings = $state<Finding[]>([]);
let scanSummary = $state<ScanSummary | null>(null);
let error = $state<string>('');
let hackerThought = $state<string>('');
let barrelRoll = $state<boolean>(false);

let eventSource: EventSource | null = null;

// ── Public Getters ──

export function getStatus() { return status; }
export function getCurrentAction() { return currentAction; }
export function getFindings() { return findings; }
export function getScanSummary() { return scanSummary; }
export function getError() { return error; }
export function getHackerThought() { return hackerThought; }
export function getBarrelRoll() { return barrelRoll; }

// ── Core API ──

/**
 * Start a scan on the given target path.
 * Creates a workspace, starts a recon session, and connects to SSE.
 */
export async function startScan(targetPath: string): Promise<void> {
	if (targetPath.trim().toLowerCase() === 'do a barrel roll') {
		barrelRoll = true;
		setTimeout(() => {
			barrelRoll = false;
		}, 2000);
		return;
	}

	// Reset
	findings = [];
	scanSummary = null;
	error = '';
	hackerThought = '';
	currentAction = 'Initializing scan...';
	status = 'scanning';
	disconnect();

	try {
		// Single stateless endpoint — no workspace/session dance.
		const res = await fetch(`${API_BASE}/api/scan`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ target_path: targetPath, role: 'recon' })
		});

		if (!res.ok) {
			const body = await res.json().catch(() => ({ error: res.statusText }));
			throw new Error(body.error || `HTTP ${res.status}`);
		}

		const { session_id } = await res.json();
		currentAction = 'Connecting to agent stream...';

		connectSSE(session_id);
	} catch (err: any) {
		error = err.message || 'Failed to start scan';
		status = 'idle';
	}
}

/**
 * Reset to idle state for a new scan.
 */
export function reset(): void {
	disconnect();
	status = 'idle';
	currentAction = '';
	hackerThought = '';
	findings = [];
	scanSummary = null;
	error = '';
}

// ── SSE Connection ──

function connectSSE(sessionId: string): void {
	disconnect();

	const url = `${API_BASE}/api/sessions/${sessionId}/stream`;
	eventSource = new EventSource(url);

	const events = [
		'connected', 'session_start', 'thinking', 'thought',
		'tool_call', 'tool_result', 'tool_error', 'tool_use',
		'completed', 'error', 'session_end', 'finding_reported', 'scan_summary'
	];

	for (const eventName of events) {
		eventSource.addEventListener(eventName, (e: MessageEvent) => {
			let data: Record<string, string> = {};
			try {
				data = JSON.parse(e.data);
			} catch {
				data = { raw: e.data };
			}
			handleEvent(eventName, data);
		});
	}

	eventSource.onerror = () => {
		if (status === 'scanning') {
			currentAction = 'Connection lost, reconnecting...';
		}
	};
}

function disconnect(): void {
	if (eventSource) {
		eventSource.close();
		eventSource = null;
	}
}

// ── Event Handler ──

function handleEvent(event: string, data: Record<string, string>): void {
	switch (event) {
		case 'tool_call': {
			const tool = data.tool || '';
			if (tool === 'read_file') {
				try {
					const args = JSON.parse(data.args || '{}');
					currentAction = `Decrypting source code: ${args.path || 'file'}`;
				} catch {
					currentAction = 'Decrypting source code...';
				}
			} else if (tool === 'list_directory') {
				try {
					const args = JSON.parse(data.args || '{}');
					currentAction = `Mapping directory topology: ${args.path || 'directory'}`;
				} catch {
					currentAction = 'Mapping directory topology...';
				}
			} else if (tool === 'search_code') {
				currentAction = 'Sniffing codebase for vulnerabilities...';
			} else if (tool === 'grep_search') {
				try {
					const args = JSON.parse(data.args || '{}');
					currentAction = `> Scanning radar for pattern: ${args.pattern || 'target'}...`;
				} catch {
					currentAction = '> Scanning radar for pattern...';
				}
			} else if (tool === 'find_secrets') {
				currentAction = 'Hunting for secrets...';
			} else if (tool === 'git_blame') {
				currentAction = 'Analyzing git history...';
			} else {
				currentAction = `Running ${tool}...`;
			}
			break;
		}

		case 'tool_use': {
			if (data.tool === 'report_finding' && data.input) {
				try {
					const f = JSON.parse(data.input);
					pushFinding(f);
				} catch {
					// Ignore parse failures
				}
			}
			break;
		}

		case 'finding_reported': {
			pushFinding({
				title: data.title || 'Untitled Finding',
				severity: data.severity || 'info',
				file_path: data.file || '',
				description: data.desc || '',
				evidence: data.evidence || ''
			});
			break;
		}

		case 'scan_summary': {
			scanSummary = {
				overall_risk: data.overall_risk || 'Unknown',
				summary: data.summary || '',
				attack_chain: data.attack_chain || ''
			};
			break;
		}

		case 'thinking':
			currentAction = 'Analyzing codebase...';
			break;

		case 'thought': {
			// Extract text string from standard provider structures, or fallback to raw
			let text = '';
			if (typeof data === 'string') {
				text = data;
			} else {
				text = data.text || data.thought || data.content || data.raw || '';
				if (!text && data.delta) {
					// Add type assertion to bypass ts errors on untyped data object since JSON.parse allows any shape
					const delta = data.delta as any;
					if (typeof delta === 'string') {
						text = delta;
					} else if (delta.text) {
						text = delta.text;
					}
				}
			}

			if (text) {
				hackerThought = hackerThought + text; // Force assignment for Svelte 5 reactivity
				if (hackerThought.length > 80) {
					hackerThought = hackerThought.slice(-80);
				}
			}
			break;
		}

		case 'session_start':
			currentAction = `Agent started (${data.model || 'unknown model'})...`;
			break;

		case 'completed':
		case 'session_end':
			status = 'completed';
			currentAction = '';
			disconnect();
			break;

		case 'error':
			error = data.error || 'Unknown error';
			status = 'completed';
			disconnect();
			break;
	}
}

function pushFinding(f: Partial<Finding>): void {
	const finding: Finding = {
		title: f.title || 'Untitled Finding',
		severity: f.severity || 'info',
		file_path: f.file_path || '',
		description: f.description || '',
		evidence: f.evidence || ''
	};

	// Deduplicate by title
	if (!findings.some(existing => existing.title === finding.title)) {
		findings.push(finding);
	}
}
