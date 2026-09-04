Drives a real Chrome/Chromium browser tab so you can see and interact with a live web page: something curl/fetch cannot do, because the page needs JavaScript to render, a login flow, or visual verification of what actually shows up on screen.

One tab stays open across calls within the same chat session, so a multi-step flow (navigate, then click, then read the result) all happens on the same page. Use `close` when you are done with it, or to force a fresh page.

Actions (set `action` to one of these):
- `navigate` — load `url` (must start with http:// or https://).
- `back` / `forward` — move through browser history.
- `snapshot` — list every interactive element currently on the page (links, buttons, inputs, selects, anything with a role), each with a short `ref` (e.g. `e3`). Set `full: true` to include elements scrolled out of the current viewport too. Call this after navigating or after anything that changes the page, before clicking or typing.
- `click` — click the element identified by `ref` (preferred, from a prior `snapshot`) or a CSS `selector`.
- `type` — replace the contents of the element identified by `ref` or `selector` with `text`.
- `key` — send a named key press to whatever is focused: `enter`, `tab`, `escape`, `backspace`, `delete`, `arrowup`, `arrowdown`, `arrowleft`, `arrowright`.
- `scroll` — scroll the page. Set `direction` to `up`, `down`, `left`, or `right`, and optionally `amount` in pixels (default 800).
- `eval` — run `script` (a JavaScript expression) in the page and return its value.
- `text` — return the visible text of the element identified by `ref` or `selector`.
- `html` — return the outer HTML of the element identified by `ref` or `selector`.
- `images` — list every image on the page (URL and alt text) — useful for finding something worth a closer look with `screenshot`.
- `console` — return console output (log/warn/error/info) and uncaught JavaScript exceptions seen since the session opened. Check this when a click or form submit had no visible effect, or when debugging a page's own behavior.
- `dialog` — respond to a native `alert()`/`confirm()`/`prompt()`/`beforeunload` dialog. Set `accept: true` to accept (OK), `false` to dismiss (Cancel), and `prompt_text` if accepting a `prompt()`. A dialog blocks the page — every other action times out until this answers it; `snapshot`'s output calls out any pending dialog for exactly this reason.
- `cdp` — send a raw Chrome DevTools Protocol command (`cdp_method`, `cdp_params`) for something none of the actions above cover. Escape hatch, not the common path — see https://chromedevtools.github.io/devtools-protocol/ for method names and parameter shapes.
- `screenshot` — capture a PNG of the current page. Set `full_page: true` for the whole scrollable page instead of just the viewport.
- `url` — return the current page URL.
- `close` — close the session so the next action starts a fresh browser.

Guidance:
- `ref` over `selector`: call `snapshot` first, then act on the `ref` it reports rather than guessing a CSS selector — a ref is exact, a hand-written selector can silently miss the intended element or hit the wrong one. A ref only lasts until the next navigation; if `click`/`type` reports the ref no longer exists, snapshot again.
- Prefer `text`/`html` for reading page content — they're cheap and exact. Reach for `screenshot` only when you actually need to see layout, styling, or something `text`/`html` can't capture (a canvas, an image, visual regressions, a CAPTCHA).
- `eval` runs arbitrary JavaScript with full page access — use it for reading page state (`document.title`, computed values) or triggering something no other action covers, not as a shortcut around `click`/`type` when those already do the job.
- If a click or form submission seems to do nothing, check `console` before assuming the page is broken — a swallowed JavaScript error is a common, invisible cause.
- A missing browser binary or a launch failure comes back as an error from the first call that needs a session; there is nothing to configure from your side beyond retrying, since this is a host environment issue.
