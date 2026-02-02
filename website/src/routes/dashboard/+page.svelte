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
		dotfiles_repo?: string;
	}

	let configs = $state<Config[]>([]);
	let loading = $state(true);
	let showModal = $state(false);
	let editingSlug = $state('');
	let saving = $state(false);
	let error = $state('');
	let toast = $state('');

	let formData = $state({
		name: '',
		description: '',
		base_preset: 'developer',
		is_public: true,
		alias: '',
		packages: [] as string[],
		custom_script: '',
		dotfiles_repo: ''
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

	interface SearchResult {
		name: string;
		desc: string;
		type: 'formula' | 'cask';
	}

	let selectedPackages = $state(new Set<string>());
	let presetExpanded = $state(false);
	let packageSearch = $state('');
	let searchResults = $state<SearchResult[]>([]);
	let searchLoading = $state(false);
	let searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;

	async function searchHomebrew(query: string) {
		if (query.length < 2) {
			searchResults = [];
			return;
		}

		searchLoading = true;
		try {
			const response = await fetch(`/api/homebrew/search?q=${encodeURIComponent(query)}`);
			const data = await response.json();
			searchResults = data.results || [];
		} catch (e) {
			console.error('Search failed:', e);
			searchResults = [];
		} finally {
			searchLoading = false;
		}
	}

	function handleSearchInput(value: string) {
		packageSearch = value;
		if (searchDebounceTimer) {
			clearTimeout(searchDebounceTimer);
		}
		if (value.length < 2) {
			searchResults = [];
			return;
		}
		searchDebounceTimer = setTimeout(() => {
			searchHomebrew(value);
		}, 300);
	}

	function getPresetPackages(preset: string): string[] {
		const p = PRESET_PACKAGES[preset];
		return p ? [...p.cli, ...p.cask] : [];
	}

	function getExtraPackages(): string[] {
		const presetPkgs = new Set(getPresetPackages(formData.base_preset));
		return Array.from(selectedPackages).filter((pkg) => !presetPkgs.has(pkg));
	}



	function initPackagesForPreset(preset: string) {
		const presetPkgs = getPresetPackages(preset);
		selectedPackages = new Set(presetPkgs);
	}

	function handlePresetChange(newPreset: string) {
		formData.base_preset = newPreset;
		initPackagesForPreset(newPreset);
	}

	function togglePresetPackage(pkg: string) {
		const newSet = new Set(selectedPackages);
		if (newSet.has(pkg)) {
			newSet.delete(pkg);
		} else {
			newSet.add(pkg);
		}
		selectedPackages = newSet;
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
		presetExpanded = false;
		if (config) {
			editingSlug = config.slug;
			formData = {
				name: config.name,
				description: config.description || '',
				base_preset: config.base_preset,
				is_public: config.is_public === 1,
				alias: config.alias || '',
				packages: config.packages || [],
				custom_script: config.custom_script || '',
				dotfiles_repo: config.dotfiles_repo || ''
			};
			const savedPkgs = config.packages || [];
			if (savedPkgs.length > 0) {
				selectedPackages = new Set(savedPkgs);
			} else {
				initPackagesForPreset(config.base_preset);
			}
		} else {
			editingSlug = '';
			formData = {
				name: '',
				description: '',
				base_preset: 'developer',
				is_public: true,
				alias: '',
				packages: [],
				custom_script: '',
				dotfiles_repo: ''
			};
			initPackagesForPreset('developer');
		}
		error = '';
		showModal = true;
	}

	function closeModal() {
		showModal = false;
	}

	function togglePackage(pkg: string) {
		const newSet = new Set(selectedPackages);
		if (newSet.has(pkg)) {
			newSet.delete(pkg);
		} else {
			newSet.add(pkg);
		}
		selectedPackages = newSet;
		formData.packages = Array.from(newSet);
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
		toast = 'Copied!';
		setTimeout(() => toast = '', 2000);
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
							<Button variant="danger" onclick={() => deleteConfig(config.slug)}>Delete</Button>
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
					<select class="form-select" onchange={(e) => handlePresetChange(e.currentTarget.value)} value={formData.base_preset}>
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

				<div class="form-group">
					<label class="form-label">Dotfiles Repository (Optional)</label>
					<input type="text" class="form-input" bind:value={formData.dotfiles_repo} placeholder="https://github.com/username/dotfiles" />
					<p class="form-hint">After installing packages, OpenBoot will clone this repo and deploy configs via stow.</p>
				</div>

				<div class="packages-section">
					<div class="packages-header">
						<span class="packages-title">Packages from "{formData.base_preset}" preset</span>
						<button class="expand-btn" onclick={() => presetExpanded = !presetExpanded}>
							{presetExpanded ? 'Collapse' : 'Expand'} ({selectedPackages.size} selected)
						</button>
					</div>
					<p class="packages-hint">Click to toggle. Gray = won't install.</p>
					<div class="preset-packages" class:expanded={presetExpanded}>
						{#each presetExpanded ? getPresetPackages(formData.base_preset) : getPresetPackages(formData.base_preset).slice(0, 12) as pkg}
							<button 
								class="preset-tag" 
								class:excluded={!selectedPackages.has(pkg)}
								onclick={() => togglePresetPackage(pkg)}
							>{pkg}</button>
						{/each}
						{#if !presetExpanded && getPresetPackages(formData.base_preset).length > 12}
							<button class="preset-tag more" onclick={() => presetExpanded = true}>
								+{getPresetPackages(formData.base_preset).length - 12} more
							</button>
						{/if}
					</div>
				</div>

			<div class="packages-section">
				<div class="packages-header">
					<span class="packages-title">Additional Packages</span>
					{#if getExtraPackages().length > 0}
						<span class="extra-count">{getExtraPackages().length} added</span>
					{/if}
				</div>
				{#if getExtraPackages().length > 0}
					<div class="selected-extras">
						{#each getExtraPackages() as pkg}
							<button type="button" class="extra-tag" onclick={() => togglePackage(pkg)}>
								{pkg}
								<span class="remove-icon">×</span>
							</button>
						{/each}
					</div>
				{/if}
				<div class="packages-search">
					<input 
						type="text" 
						class="search-input" 
						value={packageSearch}
						oninput={(e) => handleSearchInput(e.currentTarget.value)}
						placeholder="Search Homebrew packages..." 
					/>
				</div>
				{#if searchLoading}
					<div class="search-status">Searching...</div>
				{:else if packageSearch.length >= 2 && searchResults.length === 0}
					<div class="search-status">No packages found for "{packageSearch}"</div>
				{:else if packageSearch.length >= 2}
					<div class="packages-grid">
						{#each searchResults as result}
							<button type="button" class="package-item" class:selected={selectedPackages.has(result.name)} onclick={() => togglePackage(result.name)}>
								<span class="check-indicator">{selectedPackages.has(result.name) ? '✓' : ''}</span>
								<div class="package-content">
									<div class="package-info">
										<span class="package-name">{result.name}</span>
										<span class="package-type">{result.type}</span>
									</div>
									{#if result.desc}
										<span class="package-desc">{result.desc.slice(0, 60)}{result.desc.length > 60 ? '...' : ''}</span>
									{/if}
								</div>
							</button>
						{/each}
					</div>
				{:else}
					<div class="search-hint">Type at least 2 characters to search Homebrew packages</div>
				{/if}
			</div>

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

	.packages-search {
		margin-bottom: 12px;
	}

	.search-input {
		width: 100%;
		padding: 10px 14px;
		background: var(--bg-tertiary);
		border: 1px solid var(--border);
		border-radius: 8px;
		color: var(--text-primary);
		font-size: 0.9rem;
		font-family: inherit;
	}

	.search-input:focus {
		outline: none;
		border-color: var(--accent);
	}

	.search-input::placeholder {
		color: var(--text-muted);
	}



	.packages-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 12px;
	}

	.packages-hint {
		font-size: 0.75rem;
		color: var(--text-muted);
		margin-bottom: 10px;
	}

	.expand-btn {
		background: none;
		border: none;
		color: var(--accent);
		font-size: 0.8rem;
		cursor: pointer;
		padding: 4px 8px;
	}

	.expand-btn:hover {
		text-decoration: underline;
	}

	.preset-packages {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		max-height: 80px;
		overflow: hidden;
		transition: max-height 0.3s ease;
	}

	.preset-packages.expanded {
		max-height: none;
	}

	.preset-tag {
		padding: 4px 10px;
		background: var(--bg-tertiary);
		border: 1px solid var(--border);
		border-radius: 4px;
		font-size: 0.75rem;
		color: var(--text-secondary);
		font-family: 'JetBrains Mono', monospace;
		cursor: pointer;
		transition: all 0.15s;
	}

	.preset-tag:hover {
		border-color: var(--accent);
	}

	.preset-tag.excluded {
		opacity: 0.4;
		text-decoration: line-through;
		background: transparent;
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

	.extra-count {
		font-size: 0.8rem;
		color: var(--accent);
	}

	.selected-extras {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		margin-bottom: 12px;
		padding: 10px;
		background: rgba(34, 197, 94, 0.05);
		border: 1px dashed var(--accent);
		border-radius: 8px;
	}

	.extra-tag {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 4px 8px;
		background: var(--accent);
		color: #000;
		border: none;
		border-radius: 4px;
		font-size: 0.75rem;
		font-family: 'JetBrains Mono', monospace;
		cursor: pointer;
		transition: all 0.15s;
	}

	.extra-tag:hover {
		background: #1a9f4a;
	}

	.remove-icon {
		font-size: 0.9rem;
		font-weight: bold;
		opacity: 0.7;
	}

	.extra-tag:hover .remove-icon {
		opacity: 1;
	}

	.search-status,
	.search-hint {
		text-align: center;
		color: var(--text-muted);
		padding: 20px;
		font-size: 0.9rem;
	}

	.packages-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: 8px;
		max-height: 300px;
		overflow-y: auto;
		padding: 4px;
	}

	.package-item {
		display: flex;
		flex-direction: row;
		align-items: flex-start;
		gap: 10px;
		padding: 10px 12px;
		background: var(--bg-tertiary);
		border: 2px solid var(--border);
		border-radius: 6px;
		cursor: pointer;
		transition: all 0.2s;
	}

	.package-item:hover {
		border-color: var(--border-hover);
		background: var(--bg-secondary);
	}

	.package-item.selected {
		background: rgba(34, 197, 94, 0.15);
		border-color: var(--accent);
	}

	.package-item {
		text-align: left;
		font-family: inherit;
	}

	.check-indicator {
		width: 20px;
		height: 20px;
		min-width: 20px;
		display: flex;
		align-items: center;
		justify-content: center;
		border: 2px solid var(--border);
		border-radius: 4px;
		font-size: 0.75rem;
		color: var(--accent);
		background: var(--bg-tertiary);
	}

	.package-item.selected .check-indicator {
		background: var(--accent);
		border-color: var(--accent);
		color: #000;
	}

	.package-content {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.package-info {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
	}

	.package-name {
		font-family: 'JetBrains Mono', monospace;
		font-size: 0.85rem;
		font-weight: 500;
	}

	.package-type {
		font-size: 0.65rem;
		padding: 2px 6px;
		background: var(--bg-secondary);
		border-radius: 3px;
		color: var(--text-muted);
		text-transform: uppercase;
	}

	.package-desc {
		font-size: 0.75rem;
		color: var(--text-muted);
		line-height: 1.3;
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
