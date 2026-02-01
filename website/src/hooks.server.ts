import type { Handle } from '@sveltejs/kit';

function generateInstallScript(username: string, slug: string, customScript: string): string {
	return `#!/bin/bash
set -e

echo "========================================"
echo "  OpenBoot - Custom Install"
echo "  Config: @${username}/${slug}"
echo "========================================"
echo ""

ARCH="$(uname -m)"
if [ "$ARCH" = "arm64" ]; then
  ARCH="arm64"
else
  ARCH="amd64"
fi

OPENBOOT_URL="https://github.com/fullstackjam/openboot/releases/latest/download/openboot-darwin-\${ARCH}"
TMPDIR="\${TMPDIR:-/tmp}"
OPENBOOT_BIN="\$TMPDIR/openboot-\$\$"

cleanup() { rm -f "\$OPENBOOT_BIN"; }
trap cleanup EXIT

echo "Downloading OpenBoot..."
curl -fsSL "\$OPENBOOT_URL" -o "\$OPENBOOT_BIN"
chmod +x "\$OPENBOOT_BIN"

echo "Using remote config: @${username}/${slug}"
"\$OPENBOOT_BIN" --user ${username}/${slug} "\$@"

${
		customScript
			? `
echo ""
echo "=== Running Custom Post-Install Script ==="
${customScript}
`
			: ''
	}

echo ""
echo "Installation complete!"
`;
}

export const handle: Handle = async ({ event, resolve }) => {
	const path = event.url.pathname;

	const shortAliasMatch = path.match(/^\/([a-z0-9-]+)$/);
	if (shortAliasMatch && !['dashboard', 'api', 'install'].includes(shortAliasMatch[1])) {
		const alias = shortAliasMatch[1];
		const env = event.platform?.env;

		if (env) {
			const config = await env.DB.prepare('SELECT c.slug, c.custom_script, u.username FROM configs c JOIN users u ON c.user_id = u.id WHERE c.alias = ? AND c.is_public = 1')
				.bind(alias)
				.first<{ slug: string; username: string; custom_script: string }>();

			if (config) {
				const script = generateInstallScript(config.username, config.slug, config.custom_script);

				return new Response(script, {
					headers: {
						'Content-Type': 'text/plain; charset=utf-8',
						'Cache-Control': 'no-cache'
					}
				});
			}
		}
	}

	return resolve(event);
};
