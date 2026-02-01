<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';
	import Button from '$lib/components/Button.svelte';
	import { auth } from '$lib/stores/auth';

	interface Config {
		id: string;
		slug: string;
		name: string;
		description: string;
		base_preset: string;
		is_public: number;
		alias: string | null;
		packages?: string[];
		custom_script?: string;
	}

	let configs = $state<Config[]>([]);
	let loading = $state(true);
	let showModal = $state(false);
	let editingSlug = $state('');
	let saving = $state(false);
	let error = $state('');

	let formData = $state({
		name: '',
		description: '',
		base_preset: 'developer',
		is_public: true,
		alias: '',
		packages: [] as string[],
		custom_script: ''
	});

	const PRESET_PACKAGES: Record<string, { cli: string[]; cask: string[] }> = {
		minimal: {
			cli: ['curl', 'wget', 'jq', 'yq', 'ripgrep', 'fd', 'bat', 'eza', 'fzf', 'zoxide', 'htop', 'btop', 'tree', 'tldr', 'gh', 'git-delta', 'lazygit', 'stow'],
			cask: ['warp', 'raycast', 'maccy', 'stats']
		},
		developer: {
			cli: ['curl', 'wget', 'jq', 'yq', 'ripgrep', 'fd', 'bat', 'eza', 'fzf', 'zoxide', 'htop', 'btop', 'tree', 'tldr', 'gh', 'git-delta', 'lazygit', 'stow', 'node', 'go', 'pnpm', 'docker', 'docker-compose', 'tmux', 'neovim', 'httpie'],
			cask: ['warp', 'raycast', 'maccy', 'stats', 'scroll-reverser', 'visual-studio-code', 'orbstack', 'google-chrome', 'arc', 'postman', 'notion']
		},
		full: {
			cli: ['curl', 'wget', 'jq', 'yq', 'ripgrep', 'fd', 'bat', 'eza', 'fzf', 'zoxide', 'htop', 'btop', 'tree', 'tldr', 'gh', 'git-delta', 'lazygit', 'stow', 'node', 'go', 'pnpm', 'docker', 'docker-compose', 'tmux', 'neovim', 'httpie', 'python', 'uv', 'rustup', 'deno', 'bun', 'kubectl', 'helm', 'k9s', 'terraform', 'awscli', 'sqlite', 'postgresql', 'redis', 'duckdb', 'ollama', 'llm'],
			cask: ['warp', 'raycast', 'maccy', 'stats', 'scroll-reverser', 'visual-studio-code', 'cursor', 'orbstack', 'google-chrome', 'arc', 'firefox', 'postman', 'proxyman', 'notion', 'obsidian', 'figma', 'iina', 'keka', 'aldente', 'rectangle']
		}
	};

	const EXTRA_PACKAGES: Record<string, string[]> = {
		CLI: ['rustup', 'python', 'uv', 'deno', 'bun', 'kubectl', 'helm', 'k9s', 'terraform', 'awscli', 'argocd', 'sqlite', 'postgresql', 'redis', 'duckdb', 'mysql', 'ollama', 'llm'],
		Apps: ['cursor', 'zed', 'iterm2', 'alacritty', 'firefox', 'microsoft-edge', 'proxyman', 'obsidian', 'slack', 'discord', 'rectangle', 'aldente', 'keka', 'iina', 'figma', 'sketch', 'imageoptim']
	};

	let currentCategory = $state(Object.keys(EXTRA_PACKAGES)[0]);
	let selectedPackages = $state(new Set<string>());

	function getPresetPackages(preset: string): string[] {
		const p = PRESET_PACKAGES[preset];
		return p ? [...p.cli, ...p.cask] : [];
	}

	function getAvailableExtras(preset: string): Record<string, string[]> {
		const included = new Set(getPresetPackages(preset));
		const result: Record<string, string[]> = {};
		for (const [cat, pkgs] of Object.entries(EXTRA_PACKAGES)) {
			const filtered = pkgs.filter(p => !included.has(p));
			if (filtered.length > 0) result[cat] = filtered;
		}
		return result;
	}

	onMount(async () => {
		await auth.check();
		if (!$auth.user && !$auth.loading) {
			goto('/api/auth/login');
			return;
		}
		await loadConfigs();
	});

	async function loadConfigs() {
		try {
			const response = await fetch('/api/configs');
			if (response.ok) {
				const data = await response.json();
				configs = data.configs;
			}
		} catch (e) {
			console.error('Failed to load configs:', e);
		} finally {
			loading = false;
		}
	}

	function openModal(config?: Config) {
		if (config) {
			editingSlug = config.slug;
			formData = {
				name: config.name,
				description: config.description || '',
				base_preset: config.base_preset,
				is_public: config.is_public === 1,
				alias: config.alias || '',
				packages: config.packages || [],
				custom_script: config.custom_script || ''
			};
			selectedPackages = new Set(config.packages || []);
		} else {
			editingSlug = '';
			formData = {
				name: '',
				description: '',
				base_preset: 'developer',
				is_public: true,
				alias: '',
				packages: [],
				custom_script: ''
			};
			selectedPackages = new Set();
		}
		error = '';
		showModal = true;
	}

	function closeModal() {
		showModal = false;
	}

	function togglePackage(pkg: string) {
		if (selectedPackages.has(pkg)) {
			selectedPackages.delete(pkg);
		} else {
			selectedPackages.add(pkg);
		}
		selectedPackages = selectedPackages;
		formData.packages = Array.from(selectedPackages);
	}

	async function saveConfig() {
		if (!formData.name) {
			error = 'Name is required';
			return;
		}

		saving = true;
		error = '';

		const url = editingSlug ? `/api/configs/${editingSlug}` : '/api/configs';
		const method = editingSlug ? 'PUT' : 'POST';

		try {
			const response = await fetch(url, {
				method,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					...formData,
					alias: formData.alias.trim() || null,
					packages: Array.from(selectedPackages)
				})
			});

			const text = await response.text();
			let result;
			try {
				result = JSON.parse(text);
			} catch {
				error = 'Server error: ' + text.substring(0, 200);
				return;
			}

			if (!response.ok) {
				error = result.error || 'Failed to save';
				return;
			}

			closeModal();
			await loadConfigs();
		} catch (e) {
			error = 'Failed to save: ' + (e as Error).message;
		} finally {
			saving = false;
		}
	}

	async function deleteConfig(slug: string) {
		if (!confirm('Are you sure you want to delete this configuration?')) return;

		try {
			await fetch(`/api/configs/${slug}`, { method: 'DELETE' });
			await loadConfigs();
		} catch (e) {
			alert('Failed to delete configuration');
		}
	}

	async function editConfig(slug: string) {
		try {
			const response = await fetch(`/api/configs/${slug}`);
			const data = await response.json();
			openModal(data.config);
		} catch (e) {
			alert('Failed to load configuration');
		}
	}

	function copyToClipboard(text: string) {
		navigator.clipboard.writeText(text);
		alert('Copied to clipboard!');
	}

	function getInstallUrl(config: Config): string {
		if (config.alias) {
			return `openboot.dev/${config.alias}`;
		}
		return `openboot.dev/${$auth.user?.username}/${config.slug}/install`;
	}
</script>

<svelte:head>
	<title>Dashboard - OpenBoot</title>
</svelte:head>

<header class="header">
	<a href="/" class="logo">OpenBoot</a>
	<div class="user-info">
		<ThemeToggle />
		<span class="username">@{$auth.user?.username || '...'}</span>
		<Button href="/api/auth/logout" variant="ghost">Logout</Button>
	</div>
</header>

<main class="container">
	{#if loading}
		<div class="loading">Loading...</div>
	{:else}
		<div class="page-header">
			<div>
				<h1 class="page-title">My Configurations</h1>
				<p class="page-subtitle">Create custom install configs for different teams or projects</p>
			</div>
			<Button variant="primary" onclick={() => openModal()}>+ New Config</Button>
		</div>

		{#if configs.length === 0}
			<div class="empty-state">
				<h3>No configurations yet</h3>
				<p>Create your first config to get a custom install URL.</p>
			</div>
		{:else}
			<div class="configs-grid">
				{#each configs as config}
					<div class="config-card">
						<div class="config-header">
							<div>
								<div class="config-name">{config.name}</div>
								<div class="config-slug">
									{#if config.alias}
										<span class="alias">/{config.alias}</span>
									{:else}
										/{config.slug}
									{/if}
								</div>
							</div>
							<span class="badge" class:public={config.is_public} class:private={!config.is_public}>
								{config.is_public ? 'Public' : 'Private'}
							</span>
						</div>
						{#if config.description}
							<p class="config-description">{config.description}</p>
						{/if}
						<div class="config-meta">
							<span class="config-meta-item">Preset: <strong>{config.base_preset}</strong></span>
						</div>
						<div class="config-url">
							<code>curl -fsSL {getInstallUrl(config)} | bash</code>
							<button class="copy-btn" onclick={() => copyToClipboard(`curl -fsSL https://${getInstallUrl(config)} | bash`)}>Copy</button>
						</div>
						<div class="config-actions">
							<Button variant="secondary" onclick={() => editConfig(config.slug)}>Edit</Button>
							{#if config.slug !== 'default'}
								<Button variant="danger" onclick={() => deleteConfig(config.slug)}>Delete</Button>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</main>

{#if showModal}
	<div class="modal-overlay" onclick={closeModal}>
		<div class="modal" onclick={(e) => e.stopPropagation()}>
			<div class="modal-header">
				<h3 class="modal-title">{editingSlug ? 'Edit Configuration' : 'New Configuration'}</h3>
				<button class="close-btn" onclick={closeModal}>&times;</button>
			</div>
			<div class="modal-body">
				{#if error}
					<div class="error-message">{error}</div>
				{/if}

				<div class="form-group">
					<label class="form-label">Name</label>
					<input type="text" class="form-input" bind:value={formData.name} placeholder="e.g. Frontend Team" />
					<p class="form-hint">Will be used as the URL slug</p>
				</div>

				<div class="form-group">
					<label class="form-label">Description</label>
					<input type="text" class="form-input" bind:value={formData.description} placeholder="Optional description" />
				</div>

				<div class="form-group">
					<label class="form-label">Base Preset</label>
					<select class="form-select" bind:value={formData.base_preset}>
						<option value="minimal">minimal - CLI essentials</option>
						<option value="developer">developer - Ready-to-code setup</option>
						<option value="full">full - Complete dev environment</option>
					</select>
				</div>

				<div class="form-group">
					<label class="checkbox-label">
						<input type="checkbox" bind:checked={formData.is_public} />
						<span>Public (anyone can use this install URL)</span>
					</label>
				</div>

				<div class="form-group">
					<label class="form-label">Short Alias (Optional)</label>
					<div class="alias-input">
						<span class="alias-prefix">openboot.dev/</span>
						<input type="text" class="form-input" bind:value={formData.alias} placeholder="e.g. myteam" />
					</div>
					<p class="form-hint">2-20 characters, lowercase letters, numbers, and dashes only.</p>
				</div>

				<div class="packages-section">
					<div class="packages-header">
						<span class="packages-title">Included in "{formData.base_preset}" preset</span>
						<span class="packages-count">{getPresetPackages(formData.base_preset).length} packages</span>
					</div>
					<div class="preset-packages">
						{#each getPresetPackages(formData.base_preset).slice(0, 12) as pkg}
							<span class="preset-tag">{pkg}</span>
						{/each}
						{#if getPresetPackages(formData.base_preset).length > 12}
							<span class="preset-tag more">+{getPresetPackages(formData.base_preset).length - 12} more</span>
						{/if}
					</div>
				</div>

				{#if Object.keys(getAvailableExtras(formData.base_preset)).length > 0}
				<div class="packages-section">
					<div class="packages-header">
						<span class="packages-title">Additional Packages</span>
						<span class="packages-count">{selectedPackages.size} selected</span>
					</div>
					<div class="category-tabs">
						{#each Object.keys(getAvailableExtras(formData.base_preset)) as cat}
							<button class="category-tab" class:active={cat === currentCategory} onclick={() => (currentCategory = cat)}>
								{cat}
							</button>
						{/each}
					</div>
					<div class="packages-grid">
						{#each getAvailableExtras(formData.base_preset)[currentCategory] || [] as pkg}
							<label class="package-item" class:selected={selectedPackages.has(pkg)}>
								<input type="checkbox" checked={selectedPackages.has(pkg)} onchange={() => togglePackage(pkg)} />
								<span class="package-name">{pkg}</span>
							</label>
						{/each}
					</div>
				</div>
				{/if}

				<div class="form-group">
					<label class="form-label">Custom Post-Install Script (Optional)</label>
					<textarea class="form-textarea" bind:value={formData.custom_script} placeholder="#!/bin/bash&#10;# Commands to run after installation"></textarea>
				</div>
			</div>
			<div class="modal-footer">
				<Button variant="secondary" onclick={closeModal}>Cancel</Button>
				<Button variant="primary" onclick={saveConfig}>{saving ? 'Saving...' : 'Save'}</Button>
			</div>
		</div>
	</div>
{/if}

<style>
	.header {
		background: var(--bg-secondary);
		border-bottom: 1px solid var(--border);
		padding: 16px 24px;
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.logo {
		font-size: 1.25rem;
		font-weight: 600;
		color: var(--accent);
	}

	.user-info {
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.username {
		color: var(--text-secondary);
		font-size: 0.9rem;
	}

	.container {
		max-width: 1000px;
		margin: 0 auto;
		padding: 40px 24px;
	}

	.loading {
		display: flex;
		justify-content: center;
		align-items: center;
		padding: 60px;
		color: var(--text-muted);
	}

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 32px;
	}

	.page-title {
		font-size: 1.5rem;
		font-weight: 600;
	}

	.page-subtitle {
		color: var(--text-secondary);
		font-size: 0.95rem;
		margin-top: 4px;
	}

	.empty-state {
		text-align: center;
		padding: 60px 20px;
		color: var(--text-secondary);
	}

	.empty-state h3 {
		font-size: 1.25rem;
		margin-bottom: 8px;
		color: var(--text-primary);
	}

	.configs-grid {
		display: grid;
		gap: 16px;
	}

	.config-card {
		background: var(--bg-secondary);
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 20px;
		transition: all 0.2s;
	}

	.config-card:hover {
		border-color: var(--border-hover);
	}

	.config-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		margin-bottom: 12px;
	}

	.config-name {
		font-size: 1.1rem;
		font-weight: 600;
	}

	.config-slug {
		font-family: 'JetBrains Mono', monospace;
		font-size: 0.8rem;
		color: var(--text-muted);
		margin-top: 2px;
	}

	.config-slug .alias {
		color: var(--accent);
	}

	.config-description {
		color: var(--text-secondary);
		font-size: 0.9rem;
		margin-bottom: 16px;
	}

	.config-meta {
		display: flex;
		gap: 16px;
		margin-bottom: 16px;
	}

	.config-meta-item {
		font-size: 0.85rem;
		color: var(--text-muted);
	}

	.config-meta-item strong {
		color: var(--text-secondary);
	}

	.config-url {
		background: var(--bg-tertiary);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 12px;
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 16px;
		gap: 12px;
	}

	.config-url code {
		font-family: 'JetBrains Mono', monospace;
		font-size: 0.8rem;
		color: var(--accent);
		word-break: break-all;
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

	.config-actions {
		display: flex;
		gap: 8px;
	}

	.badge {
		display: inline-block;
		padding: 2px 8px;
		font-size: 0.7rem;
		border-radius: 4px;
		text-transform: uppercase;
		font-weight: 600;
	}

	.badge.public {
		background: rgba(34, 197, 94, 0.2);
		color: var(--accent);
	}

	.badge.private {
		background: rgba(239, 68, 68, 0.2);
		color: var(--danger);
	}

	.modal-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.8);
		display: flex;
		justify-content: center;
		align-items: center;
		z-index: 1000;
		padding: 20px;
	}

	.modal {
		background: var(--bg-secondary);
		border: 1px solid var(--border);
		border-radius: 16px;
		width: 100%;
		max-width: 600px;
		max-height: 90vh;
		overflow-y: auto;
	}

	.modal-header {
		padding: 20px 24px;
		border-bottom: 1px solid var(--border);
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.modal-title {
		font-size: 1.25rem;
		font-weight: 600;
	}

	.close-btn {
		background: none;
		border: none;
		font-size: 1.5rem;
		cursor: pointer;
		color: var(--text-muted);
		padding: 4px 8px;
	}

	.close-btn:hover {
		color: var(--text-primary);
	}

	.modal-body {
		padding: 24px;
	}

	.modal-footer {
		padding: 16px 24px;
		border-top: 1px solid var(--border);
		display: flex;
		justify-content: flex-end;
		gap: 12px;
	}

	.error-message {
		background: rgba(239, 68, 68, 0.1);
		border: 1px solid var(--danger);
		color: var(--danger);
		padding: 12px;
		border-radius: 8px;
		margin-bottom: 16px;
		font-size: 0.9rem;
	}

	.form-group {
		margin-bottom: 20px;
	}

	.form-label {
		display: block;
		font-size: 0.9rem;
		font-weight: 500;
		margin-bottom: 8px;
		color: var(--text-secondary);
	}

	.form-input,
	.form-select,
	.form-textarea {
		width: 100%;
		padding: 10px 14px;
		background: var(--bg-tertiary);
		border: 1px solid var(--border);
		border-radius: 8px;
		color: var(--text-primary);
		font-size: 0.95rem;
		font-family: inherit;
	}

	.form-input:focus,
	.form-select:focus,
	.form-textarea:focus {
		outline: none;
		border-color: var(--accent);
	}

	.form-textarea {
		min-height: 120px;
		resize: vertical;
		font-family: 'JetBrains Mono', monospace;
		font-size: 0.85rem;
	}

	.form-hint {
		font-size: 0.8rem;
		color: var(--text-muted);
		margin-top: 6px;
	}

	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 8px;
		cursor: pointer;
	}

	.checkbox-label input {
		width: 16px;
		height: 16px;
		accent-color: var(--accent);
	}

	.alias-input {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.alias-prefix {
		color: var(--text-muted);
		white-space: nowrap;
	}

	.packages-section {
		margin-top: 24px;
	}

	.packages-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 12px;
	}

	.preset-packages {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}

	.preset-tag {
		padding: 4px 10px;
		background: var(--bg-tertiary);
		border: 1px solid var(--border);
		border-radius: 4px;
		font-size: 0.75rem;
		color: var(--text-secondary);
		font-family: 'JetBrains Mono', monospace;
	}

	.preset-tag.more {
		background: transparent;
		border-style: dashed;
		color: var(--text-muted);
	}

	.packages-title {
		font-size: 1rem;
		font-weight: 500;
	}

	.packages-count {
		font-size: 0.85rem;
		color: var(--text-muted);
	}

	.category-tabs {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
		margin-bottom: 16px;
	}

	.category-tab {
		padding: 6px 12px;
		background: var(--bg-tertiary);
		border: 1px solid var(--border);
		border-radius: 6px;
		font-size: 0.85rem;
		cursor: pointer;
		transition: all 0.2s;
		color: var(--text-primary);
	}

	.category-tab:hover {
		border-color: var(--border-hover);
	}

	.category-tab.active {
		background: var(--accent);
		color: #000;
		border-color: var(--accent);
	}

	.packages-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
		gap: 8px;
		max-height: 300px;
		overflow-y: auto;
		padding: 4px;
	}

	.package-item {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 12px;
		background: var(--bg-tertiary);
		border: 1px solid var(--border);
		border-radius: 6px;
		cursor: pointer;
		transition: all 0.2s;
	}

	.package-item:hover {
		border-color: var(--border-hover);
	}

	.package-item.selected {
		background: rgba(34, 197, 94, 0.1);
		border-color: var(--accent);
	}

	.package-item input {
		width: 16px;
		height: 16px;
		accent-color: var(--accent);
	}

	.package-name {
		font-family: 'JetBrains Mono', monospace;
		font-size: 0.8rem;
	}

	@media (max-width: 600px) {
		.page-header {
			flex-direction: column;
			align-items: flex-start;
			gap: 16px;
		}

		.config-actions {
			flex-wrap: wrap;
		}

		.packages-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
