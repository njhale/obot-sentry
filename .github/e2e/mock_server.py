import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

# Behavior is picked by the first path segment of SERVER_URL:
#   /reject -> every request 401
#   /limit  -> key check 204, enrollment 403 (device limit)
#   /down   -> every request 503
#   /old    -> key check 403 (a server without the check), enrollment and scans succeed
#   /ok     -> key check 204, enrollment and scans succeed
# Every request is appended to mock-requests.log so the test can tell what the agent called.
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.record()
        path = self.path
        if path.startswith("/reject/"):
            self.reply(401, "unauthorized", "text/plain")
        elif path.startswith("/down/"):
            self.reply(503, "unavailable", "text/plain")
        elif path.startswith("/old/"):
            self.reply(403, "forbidden", "text/plain")
        elif path.endswith("/mdm/enroll/check") and (path.startswith("/ok/") or path.startswith("/limit/")):
            self.reply(204, "", "text/plain")
        else:
            self.reply(404, "not found", "text/plain")

    def do_POST(self):
        self.record()
        length = int(self.headers.get("Content-Length") or 0)
        self.rfile.read(length)
        path = self.path
        if path.startswith("/reject/"):
            self.reply(401, "unauthorized", "text/plain")
        elif path.startswith("/limit/"):
            self.reply(403, "Unable to enroll your device. Please contact your administrator.", "text/plain")
        elif path.startswith("/down/"):
            self.reply(503, "unavailable", "text/plain")
        elif path.endswith("/mdm/enroll") and (path.startswith("/ok/") or path.startswith("/old/")):
            body = {"device": {"id": 1, "deviceID": "e2e", "mdmConfigurationID": 7, "enrolledAt": "2026-11-04T00:00:00Z"}}
            self.reply(201, json.dumps(body), "application/json")
        elif path.startswith("/ok/") or path.startswith("/old/"):
            self.reply(201, json.dumps({"receivedAt": "2026-11-04T00:00:00Z"}), "application/json")
        else:
            self.reply(404, "not found", "text/plain")

    def record(self):
        with open("mock-requests.log", "a") as f:
            f.write(f"{self.command} {self.path}\n")

    def reply(self, code, body, content_type):
        data = body.encode()
        self.send_response(code)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, fmt, *args):
        sys.stderr.write("mock: " + (fmt % args) + "\n")
        sys.stderr.flush()


ThreadingHTTPServer(("127.0.0.1", 18080), Handler).serve_forever()
