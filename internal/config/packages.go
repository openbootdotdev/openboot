package config

type Package struct {
	Name        string
	Description string
	IsCask      bool
}

type Category struct {
	Name     string
	Icon     string
	Packages []Package
}

var Categories = []Category{
	{
		Name: "Essential",
		Icon: "⚡",
		Packages: []Package{
			{Name: "curl", Description: "Transfer data with URLs"},
			{Name: "wget", Description: "Network downloader"},
			{Name: "jq", Description: "JSON processor"},
			{Name: "yq", Description: "YAML processor"},
			{Name: "ripgrep", Description: "Fast grep alternative"},
			{Name: "fd", Description: "Fast find alternative"},
			{Name: "bat", Description: "Cat with syntax highlighting"},
			{Name: "eza", Description: "Modern ls replacement"},
			{Name: "fzf", Description: "Fuzzy finder"},
			{Name: "zoxide", Description: "Smarter cd command"},
			{Name: "htop", Description: "Interactive process viewer"},
			{Name: "btop", Description: "Resource monitor"},
			{Name: "tree", Description: "Directory tree viewer"},
			{Name: "tldr", Description: "Simplified man pages"},
		},
	},
	{
		Name: "Git & GitHub",
		Icon: "🔀",
		Packages: []Package{
			{Name: "gh", Description: "GitHub CLI"},
			{Name: "git-delta", Description: "Better git diff"},
			{Name: "lazygit", Description: "Terminal UI for git"},
			{Name: "stow", Description: "Symlink farm manager"},
		},
	},
	{
		Name: "Development",
		Icon: "🛠",
		Packages: []Package{
			{Name: "node", Description: "JavaScript runtime"},
			{Name: "go", Description: "Go programming language"},
			{Name: "rustup", Description: "Rust toolchain"},
			{Name: "python", Description: "Python 3"},
			{Name: "uv", Description: "Fast Python package manager"},
			{Name: "deno", Description: "Secure JS/TS runtime"},
			{Name: "bun", Description: "Fast JS runtime & bundler"},
			{Name: "pnpm", Description: "Fast npm alternative"},
			{Name: "tmux", Description: "Terminal multiplexer"},
			{Name: "neovim", Description: "Modern Vim"},
		},
	},
	{
		Name: "DevOps",
		Icon: "☁️",
		Packages: []Package{
			{Name: "docker", Description: "Container runtime"},
			{Name: "docker-compose", Description: "Multi-container Docker"},
			{Name: "kubectl", Description: "Kubernetes CLI"},
			{Name: "helm", Description: "Kubernetes package manager"},
			{Name: "k9s", Description: "Kubernetes TUI"},
			{Name: "terraform", Description: "Infrastructure as code"},
			{Name: "awscli", Description: "AWS CLI"},
			{Name: "argocd", Description: "GitOps for Kubernetes"},
		},
	},
	{
		Name: "Database",
		Icon: "🗄",
		Packages: []Package{
			{Name: "sqlite", Description: "Embedded SQL database"},
			{Name: "postgresql", Description: "PostgreSQL client"},
			{Name: "redis", Description: "Redis CLI"},
			{Name: "duckdb", Description: "Analytical SQL database"},
			{Name: "mysql", Description: "MySQL client"},
		},
	},
	{
		Name: "AI & ML",
		Icon: "🤖",
		Packages: []Package{
			{Name: "ollama", Description: "Run LLMs locally"},
			{Name: "llm", Description: "CLI for LLMs"},
		},
	},
	{
		Name: "Editors",
		Icon: "📝",
		Packages: []Package{
			{Name: "visual-studio-code", Description: "VS Code", IsCask: true},
			{Name: "cursor", Description: "AI-powered editor", IsCask: true},
			{Name: "zed", Description: "High-performance editor", IsCask: true},
			{Name: "windsurf", Description: "AI-native IDE", IsCask: true},
		},
	},
	{
		Name: "Browsers",
		Icon: "🌐",
		Packages: []Package{
			{Name: "google-chrome", Description: "Chrome browser", IsCask: true},
			{Name: "arc", Description: "Arc browser", IsCask: true},
			{Name: "firefox", Description: "Firefox browser", IsCask: true},
			{Name: "microsoft-edge", Description: "Edge browser", IsCask: true},
		},
	},
	{
		Name: "Terminals",
		Icon: "💻",
		Packages: []Package{
			{Name: "warp", Description: "Modern terminal", IsCask: true},
			{Name: "iterm2", Description: "Terminal emulator", IsCask: true},
			{Name: "alacritty", Description: "GPU-accelerated terminal", IsCask: true},
		},
	},
	{
		Name: "Productivity",
		Icon: "📋",
		Packages: []Package{
			{Name: "raycast", Description: "Launcher & productivity", IsCask: true},
			{Name: "maccy", Description: "Clipboard manager", IsCask: true},
			{Name: "notion", Description: "Notes & docs", IsCask: true},
			{Name: "obsidian", Description: "Knowledge base", IsCask: true},
			{Name: "slack", Description: "Team communication", IsCask: true},
			{Name: "discord", Description: "Community chat", IsCask: true},
			{Name: "telegram", Description: "Messaging", IsCask: true},
		},
	},
	{
		Name: "Utilities",
		Icon: "🔧",
		Packages: []Package{
			{Name: "stats", Description: "System monitor in menubar", IsCask: true},
			{Name: "scroll-reverser", Description: "Reverse scroll direction", IsCask: true},
			{Name: "rectangle", Description: "Window management", IsCask: true},
			{Name: "aldente", Description: "Battery charge limiter", IsCask: true},
			{Name: "keka", Description: "File archiver", IsCask: true},
			{Name: "iina", Description: "Modern media player", IsCask: true},
		},
	},
	{
		Name: "Design",
		Icon: "🎨",
		Packages: []Package{
			{Name: "figma", Description: "Design tool", IsCask: true},
			{Name: "sketch", Description: "Vector design", IsCask: true},
			{Name: "imageoptim", Description: "Image compression", IsCask: true},
		},
	},
	{
		Name: "API & Debug",
		Icon: "🔍",
		Packages: []Package{
			{Name: "httpie", Description: "HTTP client"},
			{Name: "postman", Description: "API platform", IsCask: true},
			{Name: "proxyman", Description: "HTTP debugging", IsCask: true},
			{Name: "orbstack", Description: "Docker & Linux VMs", IsCask: true},
		},
	},
}

func GetPackagesForPreset(presetName string) map[string]bool {
	selected := make(map[string]bool)

	preset, ok := Presets[presetName]
	if !ok {
		return selected
	}

	for _, pkg := range preset.CLI {
		selected[pkg] = true
	}
	for _, pkg := range preset.Cask {
		selected[pkg] = true
	}

	return selected
}

func GetAllPackageNames() []string {
	var names []string
	for _, cat := range Categories {
		for _, pkg := range cat.Packages {
			names = append(names, pkg.Name)
		}
	}
	return names
}
