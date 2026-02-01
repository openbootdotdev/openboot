import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = async ({ platform, params }) => {
	const env = platform?.env;
	if (!env) {
		return json({ error: 'Platform env not available' }, { status: 500 });
	}

	const user = await env.DB.prepare('SELECT id, username FROM users WHERE username = ?')
		.bind(params.username)
		.first<{ id: string; username: string }>();

	if (!user) {
		return json({ error: 'User not found' }, { status: 404 });
	}

	const config = await env.DB.prepare(
		'SELECT slug, name, base_preset, packages, is_public FROM configs WHERE user_id = ? AND slug = ?'
	)
		.bind(user.id, params.slug)
		.first<{ slug: string; name: string; base_preset: string; packages: string; is_public: number }>();

	if (!config) {
		return json({ error: 'Config not found' }, { status: 404 });
	}

	if (!config.is_public) {
		return json({ error: 'Config is private' }, { status: 403 });
	}

	const packages = JSON.parse(config.packages || '[]');

	return json({
		username: user.username,
		slug: config.slug,
		name: config.name,
		preset: config.base_preset,
		packages: packages
	});
};
