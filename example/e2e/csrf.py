# CSRF from a sibling subdomain ("cookie tossing") in a real browser.
#
# The attacker controls evil.example.test, the app runs on app.example.test.
# They fetch a genuine signed XSRF token for themselves, plant it as a
# Domain=example.test cookie (path=/users so it is sent first), then make the
# logged-in victim auto-submit a form with the same _token. A signed
# double-submit token alone accepts this; the Origin / Sec-Fetch-Site check
# must reject it with 419.
#
#   APP_ENV=production APP_KEY=x ./app &
#   python3 e2e/csrf.py
import re, sys, threading, urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer
from playwright.sync_api import sync_playwright, expect

APP = "http://app.example.test:8080"
EVIL = "http://evil.example.test:9999"
VICTIM_EMAIL = "pwned-by-csrf@example.test"

# 1. Attacker obtains a validly signed token (tokens are not tied to a user).
res = urllib.request.urlopen("http://127.0.0.1:8080/login")
tok = re.search(r"XSRF-TOKEN=([^;]+)", res.headers.get("Set-Cookie", "")).group(1)

EVIL_PAGE = f"""<!doctype html><body>
<form id=f method=post action="{APP}/users">
  <input name=_token value="{tok}">
  <input name=name value="CSRF victim">
  <input name=email value="{VICTIM_EMAIL}">
  <input name=role value="admin">
</form>
<script>
  document.cookie = "XSRF-TOKEN={tok}; domain=example.test; path=/users";
  document.getElementById("f").submit();
</script>"""

class Evil(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200); self.send_header("Content-Type", "text/html"); self.end_headers()
        self.wfile.write(EVIL_PAGE.encode())
    def log_message(self, *a): pass

srv = HTTPServer(("127.0.0.1", 9999), Evil)
threading.Thread(target=srv.serve_forever, daemon=True).start()

with sync_playwright() as p:
    b = p.chromium.launch(args=["--host-resolver-rules=MAP *.example.test 127.0.0.1"])
    ctx = b.new_context()
    pg = ctx.new_page()
    statuses = []
    pg.on("response", lambda r: r.request.method == "POST" and r.url.startswith(APP + "/users") and statuses.append(r.status))

    # 2. Victim logs in. Done with fetch() from the app's own page so the test
    #    doesn't need HTTPS for Inertia's history encryption; cookies land in
    #    the browser's real cookie jar.
    pg.goto(APP + "/login")
    st = pg.evaluate("""async () => {
      const tok = decodeURIComponent(document.cookie.match(/XSRF-TOKEN=([^;]+)/)[1])
      const r = await fetch('/login', {method: 'POST', headers: {'X-XSRF-TOKEN': tok},
        body: new URLSearchParams({email: 'victim@example.test', password: 'secret'})})
      return r.status }""")
    assert any(c["name"] == "demo_session" for c in ctx.cookies()), f"login failed: {st}"

    # 3. Victim opens the attacker's page on a sibling subdomain; the browser
    #    itself submits the forged form (real Origin / Sec-Fetch-Site headers).
    pg.goto(EVIL + "/"); pg.wait_for_timeout(1500)

    # 4. Did the forged request create a user?
    pg.goto(APP + "/login")
    created = VICTIM_EMAIL in pg.evaluate("fetch('/users').then(r => r.text())")
    b.close()
srv.shutdown()

blocked = not created and statuses and all(s == 419 for s in statuses)
print(("✓" if blocked else "✗"), "CSRF from sibling subdomain with planted signed token |",
      "POST status:", statuses, "| user created:", created)
sys.exit(0 if blocked else 1)
