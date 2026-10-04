import sys
from playwright.sync_api import sync_playwright, expect
B = "http://localhost:8080"
PAYLOADS = [
    '</script><script>window.__pwn=1</script>',
    '<img src=x onerror="window.__pwn=2">',
    '<!--<script>window.__pwn=3</script>',
    '"\'><svg onload=window.__pwn=4>',
    'x  </SCRIPT ><script>window.__pwn=5</script>',
]
with sync_playwright() as p:
    b = p.chromium.launch(); pg = b.new_page(); dialogs = []
    pg.on("dialog", lambda d: (dialogs.append(d.message), d.dismiss()))
    pg.goto(B + "/login"); pg.get_by_label("Email").fill("x@y.co"); pg.get_by_label("Password").fill("secret")
    pg.get_by_role("button", name="Log in").click(); expect(pg).to_have_url(B + "/")
    for i, pl in enumerate(PAYLOADS):
        pg.goto(B + "/users/create")
        pg.get_by_label("Name").fill(pl[:50]); pg.get_by_label("Email").fill(f"xss{i}@ex.co")
        pg.get_by_role("button", name="Create user").click(); expect(pg).to_have_url(B + "/users")
    # Full page load: payloads go through the <script type=application/json> page data (and SSR if on)
    pg.goto(B + "/users"); pg.wait_for_timeout(500)
    rows = pg.locator("tbody tr td.strong").all_inner_texts()[:5]
    pwn = pg.evaluate("window.__pwn")
    ok = pwn is None and not dialogs and all(any(r == pl[:50] for r in rows) for pl in PAYLOADS)
    print(("✓" if ok else "✗"), "XSS", sys.argv[1], "| __pwn =", pwn, "| rows hiển thị nguyên văn:", len([r for r in rows if r in [p[:50] for p in PAYLOADS]]), "/ 5")
    b.close()
    sys.exit(0 if ok else 1)
