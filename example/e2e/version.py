import subprocess, time, re
from playwright.sync_api import sync_playwright, expect
B = "http://localhost:8080"; EX = "."
def start(): return subprocess.Popen(["./app"], cwd=EX, stdout=open("/tmp/ginertia-version.log","a"), stderr=subprocess.STDOUT)
def css(pg): return pg.eval_on_selector('link[rel=stylesheet]', 'e => e.getAttribute("href")')
srv = start(); time.sleep(1)
with sync_playwright() as p:
    b = p.chromium.launch(); pg = b.new_page()
    reqs = []
    pg.on("response", lambda r: r.request.headers.get("x-inertia") and reqs.append((r.status, r.url, r.headers.get("x-inertia-location"))))
    pg.goto(B + "/login"); pg.get_by_label("Email").fill("a@b.co"); pg.get_by_label("Password").fill("secret")
    pg.get_by_role("button", name="Log in").click(); expect(pg).to_have_url(B + "/")
    old_css = css(pg); old_ver = pg.evaluate("document.querySelector('script[data-page]') && JSON.parse(document.querySelector('script[data-page]').textContent).version")
    pg.evaluate("window.__old = 1")

    # "Deploy" a new frontend build while the user keeps the tab open.
    open(EX + "/frontend/app.css", "a").write("\n/* deploy marker */ .deploy-marker{color:red}\n")
    subprocess.run("npm run build >/dev/null 2>&1", shell=True, cwd=EX, check=True)
    srv.terminate(); srv.wait(); srv = start(); time.sleep(1)

    pg.get_by_role("link", name="Users", exact=True).click()
    expect(pg.locator("h1")).to_have_text("Users")
    new_css = css(pg)
    assert any(s == 409 and loc for s, u, loc in reqs), reqs
    assert pg.evaluate("window.__old") is None, "no full reload"
    assert new_css != old_css, (old_css, new_css)
    print(f"✓ Asset versioning — 409 + X-Inertia-Location → full reload, CSS {old_css} → {new_css}")
    b.close()
srv.terminate()
