const INSTALL_SCRIPT_URL = 'https://raw.githubusercontent.com/fullstackjam/openboot/main/scripts/install.sh';

const SECURITY_HEADERS = {
  'X-Frame-Options': 'DENY',
  'X-Content-Type-Options': 'nosniff',
  'Referrer-Policy': 'strict-origin-when-cross-origin',
  'Permissions-Policy': 'accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()',
};

export default {
  async fetch(request, env) {
    const url = new URL(request.url);

    if (url.pathname === '/install') {
      return Response.redirect(INSTALL_SCRIPT_URL, 302);
    }

    const response = await env.ASSETS.fetch(request);

    const newHeaders = new Headers(response.headers);
    Object.entries(SECURITY_HEADERS).forEach(([key, value]) => {
      newHeaders.set(key, value);
    });

    if (url.pathname.startsWith('/assets/')) {
      newHeaders.set('Cache-Control', 'public, max-age=31536000, immutable');
    } else if (url.pathname.endsWith('.html') || url.pathname === '/') {
      newHeaders.set('Cache-Control', 'public, max-age=0, must-revalidate');
    }

    return new Response(response.body, {
      status: response.status,
      statusText: response.statusText,
      headers: newHeaders,
    });
  },
};
