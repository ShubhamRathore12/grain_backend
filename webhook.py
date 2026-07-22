#!/usr/bin/env python3
"""GitHub webhook listener for auto-deploy"""
import http.server
import subprocess
import json
import hmac
import hashlib
import os

WEBHOOK_SECRET = "grain-deploy-secret-2026"
DEPLOY_SCRIPT = "/opt/grain_backend/deploy.sh"
PORT = 9000

class WebhookHandler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path != "/webhook":
            self.send_response(404)
            self.end_headers()
            return

        content_length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(content_length)

        # Verify GitHub signature
        signature = self.headers.get("X-Hub-Signature-256", "")
        if signature:
            expected = "sha256=" + hmac.new(
                WEBHOOK_SECRET.encode(), body, hashlib.sha256
            ).hexdigest()
            if not hmac.compare_digest(signature, expected):
                self.send_response(403)
                self.end_headers()
                self.wfile.write(b"Invalid signature")
                return

        try:
            payload = json.loads(body)
            ref = payload.get("ref", "")
            if ref in ("refs/heads/main", "refs/heads/master"):
                print(f"[DEPLOY] Push to {ref} detected, deploying...")
                subprocess.Popen([DEPLOY_SCRIPT], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b"Deploying...")
            else:
                self.send_response(200)
                self.end_headers()
                self.wfile.write(f"Ignored push to {ref}".encode())
        except Exception as e:
            print(f"Error: {e}")
            self.send_response(500)
            self.end_headers()
            self.wfile.write(str(e).encode())

    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"Webhook listener active")

if __name__ == "__main__":
    server = http.server.HTTPServer(("0.0.0.0", PORT), WebhookHandler)
    print(f"Webhook listener running on port {PORT}")
    server.serve_forever()
