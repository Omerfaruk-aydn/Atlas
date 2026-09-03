Drives a real Chrome/Chromium browser tab so you can see and interact with a live web page: something curl/fetch cannot do, because the page needs JavaScript to render, a login flow, or visual verification of what actually shows up on screen.

One tab stays open across calls within the same chat session, so a multi-step flow (navigate, then click, then read the result) all happens on the same page. Use `close` when you are done with it, or to force a fresh page.

Actions (set `action` to one of these):
- `navigate` — load `url` (must start with http:// or https://).
- `click` — click the element matched by CSS `selector`.
- `type` — replace the contents of the element matched by `selector` with `text`.
- `key` — send a named key press to whatever is focused: `enter`, `tab`, `escape`, `backspace`, `delete`, `arrowup`, `arrowdown`, `arrowleft`, `arrowright`.
- `eval` — run `script` (a JavaScript expression) in the page and return its value.
- `text` — return the visible text of the element matched by `selector`.
- `html` — return the outer HTML of the element matched by `selector`.
- `screenshot` — capture a PNG of the current page. Set `full_page: true` for the whole scrollable page instead of just the viewport.
- `url` — return the current page URL.
- `close` — close the session so the next action starts a fresh browser.

Guidance:
- Prefer `text`/`html` for reading page content — they're cheap and exact. Reach for `screenshot` only when you actually need to see layout, styling, or something `text`/`html` can't capture (a canvas, an image, visual regressions).
- `eval` runs arbitrary JavaScript with full page access — use it for reading page state (`document.title`, computed values) or triggering something no selector-based action covers, not as a shortcut around `click`/`type` when those already do the job.
- A missing browser binary or a launch failure comes back as an error from the first call that needs a session; there is nothing to configure from your side beyond retrying, since this is a host environment issue.
