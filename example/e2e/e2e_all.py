import re, sys, time
from playwright.sync_api import sync_playwright, expect

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080"
errors, reqs = [], []
results = []

def ok(name, detail=""):
    results.append(name)
    print(f"✓ {name}" + (f" — {detail}" if detail else ""), flush=True)

def inertia_reqs(path=None, partial=None, purpose=None):
    out = []
    for r in reqs:
        if r["x-inertia"] != "true":
            continue
        if path and not r["url"].split("?")[0].endswith(path):
            continue
        if partial is not None and r["partial"] != partial:
            continue
        if purpose is not None and r["purpose"] != purpose:
            continue
        out.append(r)
    return out

with sync_playwright() as p:
    b = p.chromium.launch()
    ctx = b.new_context(viewport={"width": 1100, "height": 760})
    page = ctx.new_page()
    page.on("console", lambda m: m.type == "error" and errors.append(m.text))
    page.on("pageerror", lambda e: errors.append(str(e)))
    page.on("request", lambda r: reqs.append({
        "method": r.method, "url": r.url, "x-inertia": r.headers.get("x-inertia"),
        "partial": r.headers.get("x-inertia-partial-data"), "purpose": r.headers.get("purpose"),
        "intent": r.headers.get("x-inertia-infinite-scroll-merge-intent")}))
    page.on("dialog", lambda d: d.accept())

    # --- Authentication -----------------------------------------------------
    page.goto(BASE + "/users")
    expect(page).to_have_url(BASE + "/login")
    expect(page.get_by_role("heading", name="Log in")).to_be_visible()
    page.get_by_label("Email").fill("hanh@example.com")
    page.get_by_label("Password").fill("wrong")
    page.get_by_role("button", name="Log in").click()
    expect(page.get_by_text("These credentials do not match our records.")).to_be_visible()
    expect(page.get_by_label("Password")).to_have_value("")  # resetOnError
    page.get_by_label("Password").fill("secret")
    page.get_by_role("button", name="Log in").click()
    expect(page).to_have_url(BASE + "/")
    expect(page.locator(".toast")).to_have_text("Welcome back!")
    ok("Authentication", "guest → /login, sai pass báo lỗi, đúng pass vào /")
    ok("Forms (<Form> component)", "errors + resetOnError + redirect + flash")

    # --- Shared data ----------------------------------------------------------
    expect(page.locator(".who")).to_have_text("hanh")
    expect(page.locator(".brand")).to_contain_text("Ginertia Demo")
    ok("Shared data", "auth.user + appName có trên mọi page")

    page.evaluate("window.__spa = 1")

    # --- Polling --------------------------------------------------------------
    t0 = page.locator(".big.mono").nth(1).inner_text()
    n0 = len(inertia_reqs("/", partial="serverTime"))
    page.wait_for_timeout(4500)
    polls = len(inertia_reqs("/", partial="serverTime")) - n0
    assert polls >= 2, f"only {polls} poll requests"
    expect(page.locator(".big.mono").nth(1)).not_to_have_text(t0)
    page.get_by_role("button", name="Stop polling").click()
    n1 = len(inertia_reqs("/", partial="serverTime"))
    page.wait_for_timeout(2500)
    assert len(inertia_reqs("/", partial="serverTime")) == n1, "poll did not stop"
    ok("Polling", f"{polls} request/4.5s, chỉ reload serverTime, stop được")

    # --- Partial reload ---------------------------------------------------------
    t1 = page.locator(".big.mono").nth(1).inner_text()
    page.wait_for_timeout(1100)
    page.get_by_role("button", name="Partial reload").click()
    expect(page.locator(".big.mono").nth(1)).not_to_have_text(t1)
    expect(page.locator(".big.mono").first).to_have_text(re.compile(r"go1\."))
    ok("Partial reloads", "chỉ serverTime được gửi lại")

    # --- Prefetching ------------------------------------------------------------
    page.get_by_role("link", name="Users", exact=True).hover()
    page.wait_for_timeout(500)
    pre = inertia_reqs("/users", purpose="prefetch")
    assert pre, "no prefetch request on hover"
    before = len([r for r in inertia_reqs("/users") if r["partial"] is None])
    t = time.time()
    page.get_by_role("link", name="Users", exact=True).click()
    expect(page.locator("h1")).to_have_text("Users")
    dt = (time.time() - t) * 1000
    after = len([r for r in inertia_reqs("/users") if r["partial"] is None])
    assert after == before, "click re-fetched the prefetched page"
    ok("Prefetching", f"hover → prefetch, click dùng cache (không request mới, {dt:.0f}ms)")

    # --- Deferred props -----------------------------------------------------
    expect(page.locator(".stat.skeleton").first).to_be_visible()
    expect(page.locator(".stat b").first).to_have_text("20", timeout=5000)
    assert inertia_reqs("/users", partial="stats")
    ok("Deferred props", "stats load sau qua request riêng")

    # --- Forms (useForm) + Server state ----------------------------------------
    page.get_by_role("link", name="New user").click()
    page.get_by_role("button", name="Create user").click()
    expect(page.get_by_text("The name field is required.")).to_be_visible()
    page.get_by_label("Name").fill("Server State")
    page.get_by_label("Email").fill("state@example.com")
    page.get_by_role("button", name="Create user").click()
    expect(page.locator(".toast")).to_have_text("Created Server State.")
    expect(page.locator("tbody tr").first).to_contain_text("state@example.com")
    expect(page.locator(".stat b").first).to_have_text("21", timeout=5000)
    ok("Forms (useForm)", "validation errors + submit")
    ok("Server state", "list + stats lấy lại từ Go sau khi tạo (20 → 21)")

    # --- Infinite scrolling ------------------------------------------------
    page.get_by_role("link", name="Feed").click()
    expect(page.locator("article.post")).to_have_count(15)
    for want in (30, 45, 60):
        page.mouse.wheel(0, 20000)
        expect(page.locator("article.post")).to_have_count(want, timeout=5000)
    page.mouse.wheel(0, -300); page.wait_for_timeout(800)
    url_after_scroll = page.url
    assert [r["intent"] for r in reqs if "/feed?page=" in r["url"]][:3] == ["append"] * 3
    for _ in range(8):
        page.mouse.wheel(0, 20000)
        page.wait_for_timeout(250)
    expect(page.locator("article.post")).to_have_count(120, timeout=8000)
    expect(page.get_by_text("You've reached the end.")).to_be_visible()
    ids = page.eval_on_selector_all("article.post", "els => els.map(e => +e.dataset.id)")
    assert ids == list(range(1, 121)), "order/duplicates wrong"
    # Open the middle of the list directly, scroll up → previous pages prepend.
    page.goto(BASE + "/feed?page=4")
    expect(page.locator("article.post[data-id='46']")).to_be_visible()
    for _ in range(6):
        page.mouse.wheel(0, -20000)
        page.wait_for_timeout(400)
    expect(page.locator("article.post").first).to_have_attribute("data-id", "1", timeout=5000)
    ids = page.eval_on_selector_all("article.post", "els => els.map(e => +e.dataset.id)")
    assert ids == list(range(1, len(ids) + 1)) and len(ids) >= 60, ids
    assert any(r["intent"] == "prepend" for r in reqs)
    ok("Infinite scrolling", f"append 15→120 đúng thứ tự, mở ?page=4 cuộn lên thì prepend (URL khi cuộn: {url_after_scroll})")
    page.evaluate("window.__spa = 1")

    # --- Logout + history ----------------------------------------------------
    page.get_by_role("button", name="Log out").click()
    expect(page).to_have_url(BASE + "/login")
    page.go_back()
    page.wait_for_timeout(1500)
    expect(page.get_by_role("heading", name="Log in")).to_be_visible()
    assert page.locator("article.post").count() == 0, "private page visible after logout"
    ok("Authentication (logout)", "ClearHistory: bấm Back không thấy lại data cũ")

    ctx.close()
    b.close()

if errors:
    print("CONSOLE ERRORS:", *errors, sep="\n  ")
    sys.exit(1)
print(f"\nALL OK ({len(results)} checks), no console errors")
