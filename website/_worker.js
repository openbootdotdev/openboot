const INSTALL_SCRIPT_URL = 'https://raw.githubusercontent.com/fullstackjam/openboot/main/scripts/install.sh';
const GITHUB_AUTHORIZE_URL = 'https://github.com/login/oauth/authorize';
const GITHUB_TOKEN_URL = 'https://github.com/login/oauth/access_token';
const GITHUB_USER_URL = 'https://api.github.com/user';

const SECURITY_HEADERS = {
  'X-Frame-Options': 'DENY',
  'X-Content-Type-Options': 'nosniff',
  'Referrer-Policy': 'strict-origin-when-cross-origin',
};

const CORS_HEADERS = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type, Authorization',
};

function generateId() {
  return crypto.randomUUID();
}

function slugify(text) {
  return text.toLowerCase()
    .replace(/[^a-z0-9-]/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
    .substring(0, 50);
}

async function signToken(payload, secret) {
  const encoder = new TextEncoder();
  const data = JSON.stringify(payload);
  const key = await crypto.subtle.importKey(
    'raw',
    encoder.encode(secret),
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign']
  );
  const signature = await crypto.subtle.sign('HMAC', key, encoder.encode(data));
  const sigBase64 = btoa(String.fromCharCode(...new Uint8Array(signature)));
  return `${btoa(data)}.${sigBase64}`;
}

async function verifyToken(token, secret) {
  try {
    const [dataBase64, sigBase64] = token.split('.');
    if (!dataBase64 || !sigBase64) return null;
    
    const data = atob(dataBase64);
    const encoder = new TextEncoder();
    const key = await crypto.subtle.importKey(
      'raw',
      encoder.encode(secret),
      { name: 'HMAC', hash: 'SHA-256' },
      false,
      ['verify']
    );
    
    const signature = Uint8Array.from(atob(sigBase64), c => c.charCodeAt(0));
    const valid = await crypto.subtle.verify('HMAC', key, signature, encoder.encode(data));
    
    if (!valid) return null;
    
    const payload = JSON.parse(data);
    if (payload.exp && Date.now() > payload.exp) return null;
    
    return payload;
  } catch {
    return null;
  }
}

function getCookie(request, name) {
  const cookie = request.headers.get('Cookie');
  if (!cookie) return null;
  const match = cookie.match(new RegExp(`${name}=([^;]+)`));
  return match ? match[1] : null;
}

function jsonResponse(data, status = 200, headers = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { 'Content-Type': 'application/json', ...CORS_HEADERS, ...headers },
  });
}

function textResponse(text, status = 200, headers = {}) {
  return new Response(text, {
    status,
    headers: { 'Content-Type': 'text/plain', ...headers },
  });
}

async function handleGitHubLogin(env) {
  const params = new URLSearchParams({
    client_id: env.GITHUB_CLIENT_ID,
    redirect_uri: `${env.APP_URL}/api/auth/callback`,
    scope: 'read:user user:email',
    state: generateId(),
  });
  return Response.redirect(`${GITHUB_AUTHORIZE_URL}?${params}`, 302);
}

async function handleGitHubCallback(request, env) {
  const url = new URL(request.url);
  const code = url.searchParams.get('code');
  
  if (!code) {
    return Response.redirect(`${env.APP_URL}?error=no_code`, 302);
  }
  
  const tokenResponse = await fetch(GITHUB_TOKEN_URL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({
      client_id: env.GITHUB_CLIENT_ID,
      client_secret: env.GITHUB_CLIENT_SECRET,
      code,
    }),
  });
  
  const tokenData = await tokenResponse.json();
  if (tokenData.error || !tokenData.access_token) {
    return Response.redirect(`${env.APP_URL}?error=token_failed`, 302);
  }
  
  const userResponse = await fetch(GITHUB_USER_URL, {
    headers: {
      Authorization: `Bearer ${tokenData.access_token}`,
      Accept: 'application/json',
      'User-Agent': 'OpenBoot',
    },
  });
  
  const githubUser = await userResponse.json();
  if (!githubUser.id || !githubUser.login) {
    return Response.redirect(`${env.APP_URL}?error=user_failed`, 302);
  }
  
  const userId = String(githubUser.id);
  
  await env.DB.prepare(`
    INSERT INTO users (id, username, email, avatar_url, updated_at)
    VALUES (?, ?, ?, ?, datetime('now'))
    ON CONFLICT(id) DO UPDATE SET
      username = excluded.username,
      email = excluded.email,
      avatar_url = excluded.avatar_url,
      updated_at = datetime('now')
  `).bind(userId, githubUser.login, githubUser.email || '', githubUser.avatar_url || '').run();
  
  const existingConfig = await env.DB.prepare(
    'SELECT id FROM configs WHERE user_id = ? AND slug = ?'
  ).bind(userId, 'default').first();
  
  if (!existingConfig) {
    await env.DB.prepare(`
      INSERT INTO configs (id, user_id, slug, name, description, base_preset, packages)
      VALUES (?, ?, 'default', 'Default', 'My default configuration', 'developer', '[]')
    `).bind(generateId(), userId).run();
  }
  
  const token = await signToken(
    { userId, username: githubUser.login, exp: Date.now() + 7 * 24 * 60 * 60 * 1000 },
    env.JWT_SECRET
  );
  
  return new Response(null, {
    status: 302,
    headers: {
      Location: `${env.APP_URL}/dashboard`,
      'Set-Cookie': `session=${token}; Path=/; HttpOnly; Secure; SameSite=Lax; Max-Age=${7 * 24 * 60 * 60}`,
    },
  });
}

async function handleLogout(env) {
  return new Response(null, {
    status: 302,
    headers: {
      Location: env.APP_URL,
      'Set-Cookie': 'session=; Path=/; HttpOnly; Secure; SameSite=Lax; Max-Age=0',
    },
  });
}

async function getCurrentUser(request, env) {
  const token = getCookie(request, 'session');
  if (!token) return null;
  
  const payload = await verifyToken(token, env.JWT_SECRET);
  if (!payload) return null;
  
  return await env.DB.prepare(
    'SELECT id, username, email, avatar_url FROM users WHERE id = ?'
  ).bind(payload.userId).first();
}

async function handleGetUser(request, env) {
  const user = await getCurrentUser(request, env);
  if (!user) return jsonResponse({ error: 'Unauthorized' }, 401);
  return jsonResponse({ user });
}

async function handleListConfigs(request, env) {
  const user = await getCurrentUser(request, env);
  if (!user) return jsonResponse({ error: 'Unauthorized' }, 401);
  
  const { results } = await env.DB.prepare(
    'SELECT id, slug, name, description, base_preset, is_public, alias, updated_at FROM configs WHERE user_id = ? ORDER BY updated_at DESC'
  ).bind(user.id).all();
  
  return jsonResponse({ configs: results, username: user.username });
}

async function handleCreateConfig(request, env) {
  const user = await getCurrentUser(request, env);
  if (!user) return jsonResponse({ error: 'Unauthorized' }, 401);
  
  const body = await request.json();
  const { name, description, base_preset, packages, custom_script, is_public, alias } = body;
  
  if (!name) return jsonResponse({ error: 'Name is required' }, 400);
  
  const slug = slugify(name);
  if (!slug) return jsonResponse({ error: 'Invalid name' }, 400);
  
  const existing = await env.DB.prepare(
    'SELECT id FROM configs WHERE user_id = ? AND slug = ?'
  ).bind(user.id, slug).first();
  
  if (existing) return jsonResponse({ error: 'Config with this name already exists' }, 409);
  
  let cleanAlias = null;
  if (alias) {
    cleanAlias = alias.toLowerCase().replace(/[^a-z0-9-]/g, '').substring(0, 20);
    if (cleanAlias.length < 2) return jsonResponse({ error: 'Alias must be at least 2 characters' }, 400);
    if (['api', 'install', 'dashboard', 'login', 'logout'].includes(cleanAlias)) {
      return jsonResponse({ error: 'This alias is reserved' }, 400);
    }
    const aliasExists = await env.DB.prepare('SELECT id FROM configs WHERE alias = ?').bind(cleanAlias).first();
    if (aliasExists) return jsonResponse({ error: 'This alias is already taken' }, 409);
  }
  
  const configCount = await env.DB.prepare(
    'SELECT COUNT(*) as count FROM configs WHERE user_id = ?'
  ).bind(user.id).first();
  
  if (configCount.count >= 20) {
    return jsonResponse({ error: 'Maximum 20 configs per user' }, 400);
  }
  
  const id = generateId();
  await env.DB.prepare(`
    INSERT INTO configs (id, user_id, slug, name, description, base_preset, packages, custom_script, is_public, alias)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  `).bind(
    id,
    user.id,
    slug,
    name,
    description || '',
    base_preset || 'developer',
    JSON.stringify(packages || []),
    custom_script || '',
    is_public !== false ? 1 : 0,
    cleanAlias
  ).run();
  
  const installUrl = cleanAlias ? `${env.APP_URL}/${cleanAlias}` : `${env.APP_URL}/${user.username}/${slug}/install`;
  
  return jsonResponse({ 
    id, 
    slug,
    alias: cleanAlias,
    install_url: installUrl
  }, 201);
}

async function handleGetConfig(request, env, slug) {
  const user = await getCurrentUser(request, env);
  if (!user) return jsonResponse({ error: 'Unauthorized' }, 401);
  
  const config = await env.DB.prepare(
    'SELECT * FROM configs WHERE user_id = ? AND slug = ?'
  ).bind(user.id, slug).first();
  
  if (!config) return jsonResponse({ error: 'Config not found' }, 404);
  
  const installUrl = config.alias ? `${env.APP_URL}/${config.alias}` : `${env.APP_URL}/${user.username}/${slug}/install`;
  
  return jsonResponse({
    config: {
      ...config,
      packages: JSON.parse(config.packages || '[]'),
    },
    install_url: installUrl,
  });
}

async function handleUpdateConfig(request, env, slug) {
  const user = await getCurrentUser(request, env);
  if (!user) return jsonResponse({ error: 'Unauthorized' }, 401);
  
  const body = await request.json();
  const { name, description, base_preset, packages, custom_script, is_public, alias } = body;
  
  const existing = await env.DB.prepare(
    'SELECT id, alias FROM configs WHERE user_id = ? AND slug = ?'
  ).bind(user.id, slug).first();
  
  if (!existing) return jsonResponse({ error: 'Config not found' }, 404);
  
  let newSlug = slug;
  if (name && slugify(name) !== slug) {
    newSlug = slugify(name);
    const conflict = await env.DB.prepare(
      'SELECT id FROM configs WHERE user_id = ? AND slug = ? AND id != ?'
    ).bind(user.id, newSlug, existing.id).first();
    if (conflict) return jsonResponse({ error: 'Config with this name already exists' }, 409);
  }
  
  let newAlias = existing.alias;
  if (alias !== undefined) {
    if (alias === '' || alias === null) {
      newAlias = null;
    } else {
      newAlias = alias.toLowerCase().replace(/[^a-z0-9-]/g, '').substring(0, 20);
      if (newAlias.length < 2) return jsonResponse({ error: 'Alias must be at least 2 characters' }, 400);
      if (['api', 'install', 'dashboard', 'login', 'logout'].includes(newAlias)) {
        return jsonResponse({ error: 'This alias is reserved' }, 400);
      }
      const aliasExists = await env.DB.prepare('SELECT id FROM configs WHERE alias = ? AND id != ?').bind(newAlias, existing.id).first();
      if (aliasExists) return jsonResponse({ error: 'This alias is already taken' }, 409);
    }
  }
  
  await env.DB.prepare(`
    UPDATE configs SET
      slug = ?,
      name = COALESCE(?, name),
      description = COALESCE(?, description),
      base_preset = COALESCE(?, base_preset),
      packages = COALESCE(?, packages),
      custom_script = COALESCE(?, custom_script),
      is_public = COALESCE(?, is_public),
      alias = ?,
      updated_at = datetime('now')
    WHERE user_id = ? AND slug = ?
  `).bind(
    newSlug,
    name || null,
    description !== undefined ? description : null,
    base_preset || null,
    packages ? JSON.stringify(packages) : null,
    custom_script !== undefined ? custom_script : null,
    is_public !== undefined ? (is_public ? 1 : 0) : null,
    newAlias,
    user.id,
    slug
  ).run();
  
  const installUrl = newAlias ? `${env.APP_URL}/${newAlias}` : `${env.APP_URL}/${user.username}/${newSlug}/install`;
  
  return jsonResponse({ 
    success: true, 
    slug: newSlug,
    alias: newAlias,
    install_url: installUrl
  });
}

async function handleDeleteConfig(request, env, slug) {
  const user = await getCurrentUser(request, env);
  if (!user) return jsonResponse({ error: 'Unauthorized' }, 401);
  
  if (slug === 'default') {
    return jsonResponse({ error: 'Cannot delete default config' }, 400);
  }
  
  await env.DB.prepare(
    'DELETE FROM configs WHERE user_id = ? AND slug = ?'
  ).bind(user.id, slug).run();
  
  return jsonResponse({ success: true });
}

async function handlePublicInstall(username, slug, env) {
  const user = await env.DB.prepare(
    'SELECT id FROM users WHERE username = ?'
  ).bind(username).first();
  
  if (!user) return textResponse('User not found', 404);
  
  const config = await env.DB.prepare(
    'SELECT base_preset, packages, custom_script, is_public FROM configs WHERE user_id = ? AND slug = ?'
  ).bind(user.id, slug).first();
  
  if (!config) return textResponse('Config not found', 404);
  if (!config.is_public) return textResponse('Config is private', 403);
  
  const packages = JSON.parse(config.packages || '[]');
  const script = generateInstallScript(username, slug, config.base_preset, packages, config.custom_script);
  
  return new Response(script, {
    headers: { 'Content-Type': 'text/plain; charset=utf-8', 'Cache-Control': 'no-cache' },
  });
}

function generateInstallScript(username, configName, preset, packages, customScript) {
  const packagesArg = packages.length > 0 ? `--packages "${packages.join(',')}"` : '';
  
  return `#!/bin/bash
set -e

echo "╔════════════════════════════════════════════════════════════╗"
echo "║  OpenBoot - Custom Install                                 ║"
echo "║  Config: @${username}/${configName}                              "
echo "╚════════════════════════════════════════════════════════════╝"
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

echo "Running with preset: ${preset}"
"\$OPENBOOT_BIN" --preset ${preset} ${packagesArg} "\$@"

${customScript ? `
echo ""
echo "=== Running Custom Post-Install Script ==="
${customScript}
` : ''}

echo ""
echo "Installation complete!"
`;
}

async function handlePublicConfig(username, slug, env) {
  const user = await env.DB.prepare(
    'SELECT id FROM users WHERE username = ?'
  ).bind(username).first();
  
  if (!user) return jsonResponse({ error: 'User not found' }, 404);
  
  const config = await env.DB.prepare(
    'SELECT name, description, base_preset, packages, is_public FROM configs WHERE user_id = ? AND slug = ?'
  ).bind(user.id, slug).first();
  
  if (!config) return jsonResponse({ error: 'Config not found' }, 404);
  if (!config.is_public) return jsonResponse({ error: 'Config is private' }, 403);
  
  return jsonResponse({
    username,
    slug,
    name: config.name,
    description: config.description,
    preset: config.base_preset,
    packages: JSON.parse(config.packages || '[]'),
  });
}

async function handleUserProfile(username, env) {
  const user = await env.DB.prepare(
    'SELECT username, avatar_url FROM users WHERE username = ?'
  ).bind(username).first();
  
  if (!user) return jsonResponse({ error: 'User not found' }, 404);
  
  const { results } = await env.DB.prepare(
    'SELECT slug, name, description, base_preset FROM configs WHERE user_id = (SELECT id FROM users WHERE username = ?) AND is_public = 1'
  ).bind(username).all();
  
  return jsonResponse({
    username: user.username,
    avatar_url: user.avatar_url,
    configs: results,
  });
}

async function handleAliasInstall(alias, env) {
  const config = await env.DB.prepare(
    'SELECT c.*, u.username FROM configs c JOIN users u ON c.user_id = u.id WHERE c.alias = ? AND c.is_public = 1'
  ).bind(alias).first();
  
  if (!config) return null;
  
  const packages = JSON.parse(config.packages || '[]');
  const script = generateInstallScript(config.username, alias, config.base_preset, packages, config.custom_script);
  
  return new Response(script, {
    headers: { 'Content-Type': 'text/plain; charset=utf-8', 'Cache-Control': 'no-cache' },
  });
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const path = url.pathname;
    
    if (request.method === 'OPTIONS') {
      return new Response(null, { headers: CORS_HEADERS });
    }
    
    if (path === '/install') {
      return Response.redirect(INSTALL_SCRIPT_URL, 302);
    }
    
    if (path === '/api/auth/login') return handleGitHubLogin(env);
    if (path === '/api/auth/callback') return handleGitHubCallback(request, env);
    if (path === '/api/auth/logout') return handleLogout(env);
    if (path === '/api/user') return handleGetUser(request, env);
    
    if (path === '/api/configs') {
      if (request.method === 'GET') return handleListConfigs(request, env);
      if (request.method === 'POST') return handleCreateConfig(request, env);
    }
    
    const configMatch = path.match(/^\/api\/configs\/([a-z0-9-]+)$/);
    if (configMatch) {
      const slug = configMatch[1];
      if (request.method === 'GET') return handleGetConfig(request, env, slug);
      if (request.method === 'PUT') return handleUpdateConfig(request, env, slug);
      if (request.method === 'DELETE') return handleDeleteConfig(request, env, slug);
    }
    
    const shortMatch = path.match(/^\/([a-z0-9-]+)$/);
    if (shortMatch && !['dashboard', 'api', 'install'].includes(shortMatch[1])) {
      const aliasResult = await handleAliasInstall(shortMatch[1], env);
      if (aliasResult) return aliasResult;
    }
    
    const installMatch = path.match(/^\/([a-zA-Z0-9_-]+)\/([a-z0-9-]+)\/install$/);
    if (installMatch) {
      return handlePublicInstall(installMatch[1], installMatch[2], env);
    }
    
    const publicConfigMatch = path.match(/^\/([a-zA-Z0-9_-]+)\/([a-z0-9-]+)\/config$/);
    if (publicConfigMatch) {
      return handlePublicConfig(publicConfigMatch[1], publicConfigMatch[2], env);
    }
    
    const response = await env.ASSETS.fetch(request);
    const newHeaders = new Headers(response.headers);
    Object.entries(SECURITY_HEADERS).forEach(([key, value]) => newHeaders.set(key, value));
    
    if (path.startsWith('/assets/')) {
      newHeaders.set('Cache-Control', 'public, max-age=31536000, immutable');
    } else {
      newHeaders.set('Cache-Control', 'public, max-age=0, must-revalidate');
    }
    
    return new Response(response.body, {
      status: response.status,
      statusText: response.statusText,
      headers: newHeaders,
    });
  },
};
