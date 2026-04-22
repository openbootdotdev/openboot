#!/usr/bin/env python3
"""Minimal HTTP mock server for OpenBoot CI smoke tests.

Serves fixed config/packages responses so the CLI can run dry-run installs
in CI without a live openboot.dev instance.

Usage:
  python3 scripts/mock-server.py <port> <binary-path>

Example:
  python3 scripts/mock-server.py 18888 ./openboot
"""

import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlparse

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 18888
BINARY = sys.argv[2] if len(sys.argv) > 2 else "./openboot"

# Fixed RemoteConfig served at GET /<user>/<slug>/config
CONFIG = {
    "username": "testuser",
    "slug": "test-config",
    "name": "Test Config",
    "preset": "minimal",
    "packages": [{"name": "git", "desc": "Version control"}],
    "casks": [],
    "taps": [],
    "npm": [],
    "dotfiles_repo": "",
    "post_install": [],
    "shell": None,
    "macos_prefs": [],
}

# Fixed packages catalog served at GET /api/packages
PACKAGES = {
    "packages": [
        {"name": "git", "desc": "Distributed version control", "category": "essential", "type": "cli"},
        {"name": "curl", "desc": "Transfer data via URLs", "category": "essential", "type": "cli"},
    ]
}

# Install script template — runs the local binary in dry-run silent mode
INSTALL_SCRIPT = """\
#!/bin/bash
set -e
main() {{
if [ ! -t 0 ] && [ -e /dev/tty ]; then
  exec < /dev/tty || true
fi
export OPENBOOT_DRY_RUN=true
export OPENBOOT_API_URL=http://localhost:{port}
{binary} install -s -u testuser/test-config
exit 0
}}
main
"""


class MockHandler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        pass  # suppress per-request logs; server start is printed to stderr

    def _send_json(self, data, status=200):
        body = json.dumps(data).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _send_text(self, text, status=200):
        body = text.encode()
        self.send_response(status)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        path = urlparse(self.path).path

        if path in ("/testuser/test-config/config", "/test-user/test-config/config"):
            self._send_json(CONFIG)
        elif path == "/api/packages":
            self._send_json(PACKAGES)
        elif path in ("/testuser/test-config/install", "/test-user/test-config/install"):
            self._send_text(INSTALL_SCRIPT.format(port=PORT, binary=BINARY))
        else:
            self._send_json({"error": "not found"}, 404)

    def do_POST(self):
        self._send_json({"error": "not found"}, 404)


if __name__ == "__main__":
    server = HTTPServer(("localhost", PORT), MockHandler)
    sys.stderr.write(f"Mock server listening on http://localhost:{PORT}\n")
    server.serve_forever()
