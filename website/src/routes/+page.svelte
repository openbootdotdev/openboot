<script lang="ts">
	import { onMount } from 'svelte';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';
	import Button from '$lib/components/Button.svelte';
	import { auth } from '$lib/stores/auth';

	let copied = $state('');

	onMount(() => {
		auth.check();
	});

	function copyCommand(command: string, id: string) {
		navigator.clipboard.writeText(command);
		copied = id;
		setTimeout(() => (copied = ''), 2000);
	}

	const presets = [
		{
			id: 'minimal',
			name: 'minimal',
			icon: '🎯',
			description: 'CLI essentials for servers, containers, or minimalists. Fast and lightweight.',
			tools: ['ripgrep', 'fd', 'bat', 'fzf', 'lazygit', 'gh']
		},
		{
			id: 'developer',
			name: 'developer',
			icon: '⚡',
			badge: 'Recommended',
			description: 'Ready-to-code Mac setup. Everything you need to start building immediately.',
			tools: ['node', 'go', 'docker', 'VS Code', 'Chrome', 'OrbStack']
		},
		{
			id: 'full',
			name: 'full',
			icon: '🌟',
			description: 'Complete dev environment with languages, DevOps, databases, and AI tools.',
			tools: ['python', 'rust', 'kubectl', 'terraform', 'ollama', 'Cursor']
		}
	];
</script>

<svelte:head>
	<title>OpenBoot - One-line macOS Development Environment Setup</title>
	<meta name="description" content="Bootstrap your Mac development environment in minutes. One command to install everything you need - Homebrew, CLI tools, GUI apps, dotfiles, and Oh-My-Zsh." />
</svelte:head>

<header class="site-header">
	<div class="header-container">
		<a href="/" class="header-logo">OpenBoot</a>
		<div class="header-right">
			<ThemeToggle />
			{#if $auth.loading}
				<span class="loading-text">...</span>
			{:else if $auth.user}
				<Button href="/dashboard" variant="secondary">
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<rect x="3" y="3" width="7" height="7" /><rect x="14" y="3" width="7" height="7" />
						<rect x="14" y="14" width="7" height="7" /><rect x="3" y="14" width="7" height="7" />
					</svg>
					Dashboard
				</Button>
			{:else}
				<Button href="/api/auth/login" variant="secondary">
					<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
						<path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
					</svg>
					Login
				</Button>
			{/if}
		</div>
	</div>
</header>

<main>
	<div class="container">
		<section class="hero">
			<pre class="ascii-logo">
 ██████╗ ██████╗ ███████╗███╗   ██╗██████╗  ██████╗  ██████╗ ████████╗
██╔═══██╗██╔══██╗██╔════╝████╗  ██║██╔══██╗██╔═══██╗██╔═══██╗╚══██╔══╝
██║   ██║██████╔╝█████╗  ██╔██╗ ██║██████╔╝██║   ██║██║   ██║   ██║   
██║   ██║██╔═══╝ ██╔══╝  ██║╚██╗██║██╔══██╗██║   ██║██║   ██║   ██║   
╚██████╔╝██║     ███████╗██║ ╚████║██████╔╝╚██████╔╝╚██████╔╝   ██║   
 ╚═════╝ ╚═╝     ╚══════╝╚═╝  ╚═══╝╚═════╝  ╚═════╝  ╚═════╝    ╚═╝   
			</pre>

			<h1>One-line macOS Development Environment Setup</h1>
			<p class="subtitle">Bootstrap your Mac in minutes. Install Homebrew, CLI tools, GUI apps, dotfiles, and Oh-My-Zsh with a single command.</p>

			<div class="features-grid">
				<div class="feature-item">Beautiful interactive TUI</div>
				<div class="feature-item">Apple Silicon & Intel support</div>
				<div class="feature-item">3 curated presets</div>
				<div class="feature-item">Oh-My-Zsh + shell aliases</div>
				<div class="feature-item">Dotfiles management</div>
				<div class="feature-item">macOS preferences tuning</div>
				<div class="feature-item">Dry-run mode for preview</div>
				<div class="feature-item">100% open source</div>
			</div>

			<div class="cta-buttons">
				<Button href="https://github.com/fullstackjam/openboot" variant="primary">
					<svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
						<path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
					</svg>
					View on GitHub
				</Button>
				<Button href="#install" variant="secondary">Get Started</Button>
			</div>

			<div class="install-command" id="install">
				<code>curl -fsSL https://openboot.dev/install | bash</code>
				<button class="copy-btn" onclick={() => copyCommand('curl -fsSL https://openboot.dev/install | bash', 'main')}>
					{copied === 'main' ? 'Copied!' : 'Copy'}
				</button>
			</div>

			<p class="privacy-note">No analytics. No telemetry. Your data stays on your machine.</p>
		</section>

		<section class="presets-section">
			<div class="section-header">
				<h2 class="section-title">Choose Your Preset</h2>
				<p class="section-subtitle">Start with a preset, customize with the interactive selector</p>
			</div>
			<div class="presets-container">
				{#each presets as preset}
					<button
						class="preset-card"
						class:featured={preset.id === 'developer'}
						class:copied={copied === preset.id}
						onclick={() => copyCommand(`curl -fsSL openboot.dev/install | bash -s -- --preset ${preset.id}`, preset.id)}
					>
						<div class="preset-icon">{preset.icon}</div>
						<div class="preset-header">
							<h3>
								{preset.name}
								{#if preset.badge}
									<span class="badge">{preset.badge}</span>
								{/if}
							</h3>
						</div>
						<p>{preset.description}</p>
						<div class="preset-tools">
							{#each preset.tools as tool}
								<span>{tool}</span>
							{/each}
						</div>
						<div class="command-preview">
							<span class="cmd">curl ... | bash -s -- --preset {preset.id}</span>
							<span class="hint">Click to copy</span>
						</div>
					</button>
				{/each}
			</div>

			<div class="custom-config-cta">
				{#if $auth.user}
					<p>Manage your custom configurations</p>
					<Button href="/dashboard" variant="secondary">
						<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<rect x="3" y="3" width="7" height="7" /><rect x="14" y="3" width="7" height="7" />
							<rect x="14" y="14" width="7" height="7" /><rect x="3" y="14" width="7" height="7" />
						</svg>
						Go to Dashboard
					</Button>
				{:else}
					<p>Want a custom setup for your team?</p>
					<Button href="/api/auth/login" variant="secondary">
						<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
							<path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
						</svg>
						Create Custom Config
					</Button>
				{/if}
			</div>
		</section>
	</div>
</main>

<footer>
	<div class="container">
		<div class="footer-links">
			<a href="https://github.com/fullstackjam/openboot">
				<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>
				GitHub
			</a>
			<a href="https://github.com/fullstackjam/openboot/issues">
				<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/></svg>
				Issues
			</a>
			<a href="https://github.com/fullstackjam/openboot/blob/main/README.md">
				<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
				Docs
			</a>
		</div>
		<p class="footer-text">MIT License · Made for the macOS developer community</p>
	</div>
</footer>

<style>
	.container {
		max-width: 1200px;
		margin: 0 auto;
		padding: 0 24px;
	}

	.site-header {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		background: var(--header-bg);
		backdrop-filter: blur(12px);
		border-bottom: 1px solid var(--border);
		z-index: 100;
	}

	.header-container {
		max-width: 1200px;
		margin: 0 auto;
		padding: 12px 24px;
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.header-logo {
		font-size: 1.1rem;
		font-weight: 600;
		color: var(--accent);
	}

	.header-right {
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.loading-text {
		color: var(--text-muted);
		font-size: 0.9rem;
	}

	main {
		padding: 80px 0 60px;
	}

	.hero {
		text-align: center;
		padding: 40px 0 60px;
	}

	.ascii-logo {
		font-family: 'JetBrains Mono', monospace;
		font-size: clamp(6px, 1.2vw, 10px);
		line-height: 1.2;
		color: var(--accent);
		white-space: pre;
		margin-bottom: 40px;
		overflow: hidden;
	}

	.hero h1 {
		font-size: 2.5rem;
		font-weight: 700;
		margin-bottom: 16px;
		background: linear-gradient(135deg, var(--text-primary) 0%, var(--text-secondary) 100%);
		-webkit-background-clip: text;
		-webkit-text-fill-color: transparent;
		background-clip: text;
	}

	.subtitle {
		font-size: 1.125rem;
		color: var(--text-secondary);
		max-width: 600px;
		margin: 0 auto 32px;
	}

	.features-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 12px;
		max-width: 800px;
		margin: 0 auto 40px;
		text-align: left;
	}

	.feature-item {
		display: flex;
		align-items: center;
		gap: 8px;
		color: var(--text-secondary);
		font-size: 0.9rem;
	}

	.feature-item::before {
		content: '✓';
		color: var(--accent);
		font-weight: 600;
	}

	.cta-buttons {
		display: flex;
		gap: 16px;
		justify-content: center;
		flex-wrap: wrap;
		margin-bottom: 40px;
	}

	.install-command {
		background: var(--code-bg);
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 20px 24px;
		max-width: 700px;
		margin: 0 auto 20px;
		position: relative;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
	}

	.install-command code {
		font-family: 'JetBrains Mono', monospace;
		font-size: 0.95rem;
		color: var(--accent);
	}

	.copy-btn {
		background: var(--bg-tertiary);
		border: 1px solid var(--border);
		color: var(--text-secondary);
		padding: 6px 12px;
		border-radius: 6px;
		font-size: 0.8rem;
		cursor: pointer;
		transition: all 0.2s;
		white-space: nowrap;
	}

	.copy-btn:hover {
		background: var(--border);
		color: var(--text-primary);
	}

	.privacy-note {
		text-align: center;
		color: var(--text-muted);
		font-size: 0.85rem;
		margin-bottom: 60px;
	}

	.presets-section {
		margin-bottom: 80px;
	}

	.section-header {
		text-align: center;
		margin-bottom: 40px;
	}

	.section-title {
		font-size: 1.75rem;
		font-weight: 600;
		margin-bottom: 12px;
	}

	.section-subtitle {
		color: var(--text-secondary);
		font-size: 1rem;
	}

	.presets-container {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 16px;
	}

	.preset-card {
		background: var(--bg-secondary);
		border: 1px solid var(--border);
		border-radius: 20px;
		padding: 28px;
		transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
		position: relative;
		overflow: hidden;
		cursor: pointer;
		text-align: left;
	}

	.preset-card:hover {
		border-color: var(--border-hover);
		transform: translateY(-6px);
	}

	.preset-card.featured {
		background: linear-gradient(135deg, var(--bg-secondary) 0%, rgba(34, 197, 94, 0.06) 100%);
		border-color: rgba(34, 197, 94, 0.35);
	}

	.preset-card.copied {
		border-color: var(--accent);
	}

	.preset-icon {
		width: 48px;
		height: 48px;
		border-radius: 12px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 1.5rem;
		margin-bottom: 16px;
		background: rgba(255, 255, 255, 0.05);
		border: 1px solid rgba(255, 255, 255, 0.08);
	}

	.preset-header h3 {
		font-size: 1.1rem;
		font-weight: 600;
		display: flex;
		align-items: center;
		gap: 10px;
		margin-bottom: 8px;
	}

	.badge {
		font-size: 0.65rem;
		padding: 3px 8px;
		background: var(--accent);
		color: #000;
		border-radius: 20px;
		font-weight: 600;
		text-transform: uppercase;
	}

	.preset-card p {
		color: var(--text-secondary);
		font-size: 0.875rem;
		margin-bottom: 16px;
		line-height: 1.5;
	}

	.preset-tools {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}

	.preset-tools span {
		font-family: 'JetBrains Mono', monospace;
		font-size: 0.7rem;
		padding: 4px 10px;
		background: rgba(255, 255, 255, 0.05);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: 6px;
		color: var(--text-secondary);
	}

	.command-preview {
		position: absolute;
		bottom: 0;
		left: 0;
		right: 0;
		padding: 12px 20px;
		background: linear-gradient(to top, rgba(0, 0, 0, 0.95) 0%, rgba(0, 0, 0, 0.8) 100%);
		font-family: 'JetBrains Mono', monospace;
		font-size: 0.7rem;
		color: var(--accent);
		opacity: 0;
		transform: translateY(100%);
		transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
		border-radius: 0 0 20px 20px;
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.preset-card:hover .command-preview {
		opacity: 1;
		transform: translateY(0);
	}

	.command-preview .hint {
		color: var(--text-muted);
		font-size: 0.65rem;
	}

	.custom-config-cta {
		text-align: center;
		margin-top: 32px;
	}

	.custom-config-cta p {
		color: var(--text-secondary);
		margin-bottom: 16px;
	}

	footer {
		border-top: 1px solid var(--border);
		padding: 40px 0;
		text-align: center;
	}

	.footer-links {
		display: flex;
		justify-content: center;
		gap: 24px;
		margin-bottom: 20px;
		flex-wrap: wrap;
	}

	.footer-links a {
		color: var(--text-secondary);
		font-size: 0.9rem;
		display: flex;
		align-items: center;
		gap: 6px;
		transition: color 0.2s;
	}

	.footer-links a:hover {
		color: var(--text-primary);
	}

	.footer-links svg {
		width: 18px;
		height: 18px;
	}

	.footer-text {
		color: var(--text-muted);
		font-size: 0.85rem;
	}

	@media (max-width: 768px) {
		.ascii-logo {
			font-size: 6px;
		}

		.hero h1 {
			font-size: 1.75rem;
		}

		.presets-container {
			grid-template-columns: 1fr;
		}
	}
</style>
