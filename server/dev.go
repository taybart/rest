package server

import (
	"bufio"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/taybart/log"
	lua "github.com/taybart/midnight"
)

// Dev mode (rest -s -f site.rest --dev) is for working on a site:
//
//   - __REST_DEV=1 is set (by the cli, before the file is parsed)
//   - handler `cache` is off (request block caches stay on), and files from
//     `directory` are sent w/ `Cache-Control: no-cache`, so browsers check
//     for a newer one instead of using what they have cached
//   - html rest renders gets a script that reloads the page when rest says
//     so (over /_rest/dev), and after rest restarts
//   - under `run -w` (with RUN_EVENTS_FD), rest handles file changes itself:
//     a change to the rest file, an import, or a file read() read needs a
//     new parse, so rest asks run to restart it. A .css file under
//     `directory` is swapped in place, and anything else (pages, markdown,
//     scripts, ...) reloads the page: re-rendered and morphed in, so scroll
//     and what's typed survive (interactive pages reload, their lua state is
//     per page load).
//   - a script error shows in the browser (see errorResponse), and goes away
//     once a change fixes it

// DevEventsPath is the server sent events the dev script listens to
const DevEventsPath = "/_rest/dev"

// devMorphPath serves idiomorph, for pages that don't have it (only
// interactive ones do)
const devMorphPath = "/_rest/idiomorph.js"

type devMode struct {
	sources map[string]bool // abs paths a change to restarts rest
	dir     string          // abs directory, for css paths

	mu      sync.Mutex
	clients map[chan string]bool
}

func newDevMode(c Config) *devMode {
	d := &devMode{sources: map[string]bool{}, clients: map[chan string]bool{}}
	for _, s := range c.DevSources {
		d.sources[s] = true
	}
	if c.Dir != "" {
		d.dir, _ = filepath.Abs(c.Dir)
	}
	return d
}

func (d *devMode) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+DevEventsPath, d.events)
	mux.HandleFunc("GET "+devMorphPath, func(w http.ResponseWriter, r *http.Request) {
		src, err := idiomorphSource()
		if err != nil {
			log.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/javascript")
		w.Write([]byte(src))
	})
}

// events streams "reload" and "css <path>" to a page
func (d *devMode) events(w http.ResponseWriter, r *http.Request) {
	// (streams stay open, unlike the server's write timeout)
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	// retry quickly after a restart, the page reloads when it's back
	fmt.Fprint(w, "retry: 300\n\n")
	if rc.Flush() != nil {
		return
	}

	ch := make(chan string, 8)
	d.mu.Lock()
	d.clients[ch] = true
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		delete(d.clients, ch)
		d.mu.Unlock()
	}()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			event, data, _ := strings.Cut(msg, " ")
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
			if rc.Flush() != nil {
				return
			}
		}
	}
}

func (d *devMode) broadcast(msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for ch := range d.clients {
		select {
		case ch <- msg:
		default: // (a page that isn't keeping up reloads on its next one)
		}
	}
}

// listen handles `run -w` changes (see run's fd 3 protocol), if rest runs
// under it
func (d *devMode) listen() {
	fd, err := strconv.Atoi(os.Getenv("RUN_EVENTS_FD"))
	if err != nil || fd < 3 {
		return
	}
	f := os.NewFile(uintptr(fd), "run-events")
	if _, err := f.WriteString("hello\n"); err != nil {
		return
	}
	log.Infof("dev: handling file changes from run\n")
	sc := bufio.NewScanner(f)
	batch := []string{}
	for sc.Scan() {
		line := sc.Text()
		if line != "done" {
			// change <kind> <path>
			if rest, ok := strings.CutPrefix(line, "change "); ok {
				if _, path, ok := strings.Cut(rest, " "); ok {
					batch = append(batch, path)
				}
			}
			continue
		}
		if d.handle(batch) {
			f.WriteString("restart\n")
		}
		batch = batch[:0]
	}
}

// handle reacts to a batch of changed files (relative to the working
// directory), true if rest needs a restart
func (d *devMode) handle(paths []string) bool {
	css := []string{}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if d.sources[abs] {
			log.Infof("dev: %s changed, restarting\n", p)
			return true
		}
		if url, ok := d.cssURL(abs); ok {
			css = append(css, url)
		} else {
			css = nil // (anything else reloads the page anyway)
			break
		}
	}
	if len(css) > 0 {
		log.Infof("dev: %s changed, updating styles\n", strings.Join(paths, ", "))
		for _, url := range css {
			d.broadcast("css " + url)
		}
		return false
	}
	log.Infof("dev: %s changed, reloading pages\n", strings.Join(paths, ", "))
	d.broadcast("reload")
	return false
}

// cssURL is the url of a css file under the served directory
func (d *devMode) cssURL(abs string) (string, bool) {
	if d.dir == "" || filepath.Ext(abs) != ".css" {
		return "", false
	}
	rel, err := filepath.Rel(d.dir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return "/" + filepath.ToSlash(rel), true
}

// DevErrorHeader marks a script error's response in dev mode, the dev script
// shows those over the page
const DevErrorHeader = "X-Rest-Dev-Error"

// errorResponse is a script error for the browser: a page load gets an html
// page with the error (and the dev script, which shows it and reloads once a
// change fixes it), anything else (the dev script's re-fetch, an interactive
// page's actions, a page's own fetches) gets the error as text, which the dev
// script shows over the page
func (d *devMode) errorResponse(r *http.Request, err error) Response {
	res := Response{
		Status:  http.StatusInternalServerError,
		Headers: map[string]string{DevErrorHeader: "1", "Cache-Control": "no-store"},
	}
	if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html") {
		res.Headers["Content-Type"] = "text/html; charset=utf-8"
		res.Body = []byte(fmt.Sprintf(devErrorPage,
			html.EscapeString(r.URL.Path), html.EscapeString(err.Error())))
		return d.inject(res)
	}
	res.Headers["Content-Type"] = "text/plain; charset=utf-8"
	res.Body = []byte(err.Error())
	return res
}

// devErrorPage is a page load's script error (the dev script shows it, the
// <pre> is for reading it without the script)
const devErrorPage = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>error: %s</title></head>
<body style="background:#0b0f1d;color:#e8ebf4"><pre data-rest-error>%s</pre></body>
</html>`

// inject adds the dev script to an html response
func (d *devMode) inject(res Response) Response {
	if ct, ok := headerKey(res.Headers, "Content-Type"); !ok || !strings.HasPrefix(res.Headers[ct], "text/html") {
		return res
	}
	body := string(res.Body)
	if i := strings.LastIndex(strings.ToLower(body), "</body>"); i >= 0 {
		body = body[:i] + devScript + body[i:]
	} else {
		body += devScript
	}
	res.Body = []byte(body)
	return res
}

// devScript reloads the page on "reload": static pages are re-fetched and
// morphed in (keeping scroll, focus, and inputs), interactive ones (with the
// midnight client) reload, since their lua state is per page load. "css
// <path>" swaps that stylesheet. When rest comes back after a restart, the
// event stream reconnects, and the page reloads the same way (so a read()
// page's edit, which restarts rest, is morphed in too).
//
// Errors show in an overlay (in a shadow root on <html>, so morphs and the
// page's styles leave it alone): a script error from the re-fetch (over the
// last good page, until a reload fixes it), from any fetch (ex. an
// interactive page's actions), or an error page's (see errorResponse). While
// rest is down (ex. the rest file doesn't parse), a banner says so.
var devScript = `<script data-rest-dev>(() => {
  let connected = false
  const errorHeader = '` + DevErrorHeader + `'
  const fetch = window.fetch.bind(window)

  // the overlay (made when first needed), a sleepy midnight sky
  const moon = (size) => '<svg class="moon" width="' + size + '" height="' + size + '" viewBox="0 0 48 48" aria-hidden="true">' +
    '<mask id="crescent-' + size + '"><rect width="48" height="48" fill="#fff"/><circle cx="32" cy="16" r="15" fill="#000"/></mask>' +
    '<circle cx="22" cy="26" r="17" fill="#f6f1d8" mask="url(#crescent-' + size + ')"/></svg>'
  let ui
  const overlay = () => {
    if (ui?.host.isConnected) return ui
    const host = document.createElement('rest-dev-overlay')
    const root = host.attachShadow({ mode: 'open' })
    root.innerHTML = ` + "`" + `<style>
      :host { all: initial; --bg: #0b0f1d; --card: #0e1428; --border: #1e2642; --text: #e8ebf4;
        --muted: #8f97ad; --accent: #b9c4ff; --moon: #f6f1d8; --red: #f07178; --gold: #ffcb6b;
        --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
        --sans: system-ui, -apple-system, 'Segoe UI', sans-serif }
      .error { position: fixed; inset: 0; z-index: 2147483647; overflow: auto; box-sizing: border-box;
        padding: 8vh 16px; color: var(--text); font: 15px/1.5 var(--sans);
        background: radial-gradient(ellipse at 50% 0%, rgba(120, 140, 220, .16), transparent 60%),
          rgba(11, 15, 29, .94);
        backdrop-filter: blur(3px) }
      .sky { position: fixed; top: 0; left: 0; width: 2px; height: 2px; border-radius: 50%;
        pointer-events: none; animation: twinkle 4s ease-in-out infinite alternate;
        box-shadow: 8vw 12vh #fff8, 19vw 31vh #fff6, 27vw 7vh #fffa, 36vw 22vh #fff5, 44vw 4vh #fff8,
          53vw 17vh #fff6, 61vw 9vh #fffa, 70vw 27vh #fff5, 78vw 13vh #fff8, 87vw 5vh #fff6,
          93vw 24vh #fffa, 4vw 45vh #fff5, 15vw 62vh #fff6, 24vw 81vh #fff8, 33vw 92vh #fff5,
          58vw 74vh #fff6, 67vw 88vh #fff8, 76vw 58vh #fff5, 85vw 79vh #fffa, 96vw 66vh #fff6 }
      .sky.b { animation-delay: -2s; transform: translate(3vw, 5vh) }
      @keyframes twinkle { from { opacity: .35 } to { opacity: 1 } }
      .box { position: relative; max-width: 880px; margin: 0 auto; background: var(--card);
        border: 1px solid var(--border); border-radius: 14px; padding: 28px 30px 24px;
        box-shadow: 0 20px 60px rgba(0, 0, 0, .5), 0 0 80px rgba(120, 140, 220, .08) }
      .head { display: flex; gap: 18px; align-items: center; margin-bottom: 20px }
      .sleepy { position: relative; flex: none; width: 56px; height: 56px }
      .moon { filter: drop-shadow(0 0 10px rgba(246, 241, 216, .45)) }
      .z { position: absolute; left: 40px; top: 6px; color: var(--accent); font: 600 13px var(--mono);
        opacity: 0; animation: drift 3.6s ease-in infinite }
      .z:nth-of-type(2) { animation-delay: 1.2s; font-size: 11px }
      .z:nth-of-type(3) { animation-delay: 2.4s; font-size: 9px }
      @keyframes drift { 0% { opacity: 0; transform: translate(0, 0) }
        20% { opacity: .9 } 100% { opacity: 0; transform: translate(16px, -28px) } }
      .titles { flex: 1; min-width: 0 }
      .kicker { color: var(--muted); font: 12px var(--mono); letter-spacing: .02em }
      .kicker b { color: var(--accent); font-weight: normal }
      h1 { margin: 2px 0 0; font: 600 22px/1.3 var(--sans); color: var(--text) }
      pre { margin: 0; padding: 16px 18px; background: #090d19; border: 1px solid var(--border);
        border-left: 3px solid var(--red); border-radius: 8px; overflow: auto;
        font: 13.5px/1.6 var(--mono); white-space: pre-wrap; word-break: break-word }
      .at { color: var(--gold) }
      .src { color: var(--muted) }
      .hint { color: var(--muted); font-size: 13px; margin-top: 16px }
      kbd { font: 11px var(--mono); border: 1px solid var(--border); border-radius: 4px; padding: 1px 5px }
      button { all: unset; cursor: pointer; align-self: flex-start; color: var(--muted);
        font-size: 22px; line-height: 1; padding: 2px 6px; border-radius: 6px }
      button:hover { color: var(--text); background: rgba(255, 255, 255, .06) }
      .down { position: fixed; z-index: 2147483647; bottom: 16px; left: 50%; transform: translateX(-50%);
        display: flex; gap: 10px; align-items: center; max-width: calc(100vw - 32px); box-sizing: border-box;
        background: var(--card); color: var(--text); border: 1px solid var(--border); border-radius: 999px;
        padding: 8px 18px 8px 12px; box-shadow: 0 8px 30px rgba(0, 0, 0, .45);
        font: 13px/1.4 var(--sans) }
      .down span { color: var(--muted) }
      @media (prefers-reduced-motion: reduce) { .sky, .z { animation: none } .z { opacity: .7 } }
      [hidden] { display: none !important }
    </style>
    <div class="error" hidden>
      <div class="sky"></div><div class="sky b"></div>
      <div class="box" role="alertdialog" aria-labelledby="title">
        <div class="head">
          <div class="sleepy">` + "` + moon(56) + `" + `<i class="z">z</i><i class="z">z</i><i class="z">z</i></div>
          <div class="titles">
            <div class="kicker">rest dev &middot; error in <b class="path"></b></div>
            <h1 id="title">something went bump in the night</h1>
          </div>
          <button title="dismiss (esc)" aria-label="dismiss">&times;</button>
        </div>
        <pre></pre>
        <div class="hint">fix it and save, rest will tuck the page back in &middot; <kbd>esc</kbd> to peek at the page</div>
      </div>
    </div>
    <div class="down" hidden>` + "` + moon(22) + `" + `<div>rest is asleep, waiting for it to wake up <span>(see its output)</span></div></div>` + "`" + `
    document.documentElement.append(host)
    const error = root.querySelector('.error')
    root.querySelector('button').onclick = () => { error.hidden = true }
    ui = { host, error, path: root.querySelector('.path'), pre: root.querySelector('pre'), down: root.querySelector('.down') }
    return ui
  }
  // the error, w/ positions (file.mn:2:) and source lines (line 2 -> ...) picked out
  const showError = (path, text) => {
    const o = overlay()
    o.path.textContent = path
    o.pre.replaceChildren()
    text.split('\n').forEach((line, i) => {
      if (i) o.pre.append('\n')
      const m = line.match(/^(\S+:\d+:)(.*)$/) || line.match(/^(line \d+ ->)(.*)$/)
      if (!m) return o.pre.append(line)
      const at = document.createElement('span')
      at.className = m[1].startsWith('line ') ? 'src' : 'at'
      at.textContent = m[1]
      o.pre.append(at, m[2])
    })
    o.error.hidden = false
    console.error('rest dev: error in ' + path + '\n' + text)
  }
  const hideError = () => { if (ui) ui.error.hidden = true }
  addEventListener('keydown', (e) => { if (e.key === 'Escape') hideError() })

  // a script error from any of the page's fetches (ex. an action)
  window.fetch = async (...args) => {
    const res = await fetch(...args)
    if (res.headers.has(errorHeader)) {
      const url = new URL(res.url || location.href)
      res.clone().text().then((text) => showError(url.pathname, text), () => {})
    }
    return res
  }

  // an error page (a page load's script error)
  const errorPage = document.querySelector('[data-rest-error]')
  if (errorPage) showError(location.pathname, errorPage.textContent)

  const morphLib = () => window.Idiomorph ? Promise.resolve() : new Promise((ok, fail) => {
    const s = document.createElement('script')
    s.src = '` + devMorphPath + `'
    s.onload = ok
    s.onerror = fail
    document.head.append(s)
  })
  // full reloads say why (in the console)
  const hard = (why) => {
    console.info('rest dev: full reload,', why)
    location.reload()
  }
  const reload = async () => {
    try {
      // (the midnight client: window.midnight alone can be an element with
      // id="midnight", browsers put those on window)
      if (typeof window.midnight?.rpc === 'function') return hard('interactive page (its lua state is per page load)')
      if (errorPage) return hard('the page was an error')
      const res = await fetch(location.href)
      if (res.headers.has(errorHeader)) return showError(location.pathname, await res.text())
      if (!res.ok) return hard('the page is a ' + res.status)
      const next = new DOMParser().parseFromString(await res.text(), 'text/html')
      await morphLib()
      document.title = next.title
      // keeps what's typed / checked in every form field, not just the
      // focused one (the markup's defaults usually didn't change)
      const typed = (attr, el) => !(['value', 'checked', 'selected'].includes(attr) &&
        el.matches('input, textarea, select, option'))
      Idiomorph.morph(document.body, next.body, {
        ignoreActiveValue: true,
        callbacks: { beforeAttributeUpdated: typed },
      })
      hideError()
    } catch (e) {
      console.error('rest dev:', e)
      hard('morphing failed')
    }
  }
  const css = (path) => {
    for (const link of document.querySelectorAll('link[rel=stylesheet]')) {
      const url = new URL(link.href, location.href)
      if (url.origin !== location.origin || url.pathname !== path) continue
      url.searchParams.set('_rest', Date.now())
      link.href = url.href
    }
  }
  const events = new EventSource('` + DevEventsPath + `')
  let downTimer
  events.onopen = () => {
    clearTimeout(downTimer)
    downTimer = undefined
    if (ui) ui.down.hidden = true
    if (connected) { // (rest restarted, ex. a read() page changed)
      console.info('rest dev: rest restarted, reloading')
      reload()
    }
    connected = true
  }
  // (after a moment, the stream errors when leaving the page too)
  // (errors again on every retry, so the first one starts the timer)
  events.onerror = () => {
    downTimer ??= setTimeout(() => {
      if (events.readyState !== EventSource.OPEN) overlay().down.hidden = false
    }, 1000)
  }
  events.addEventListener('reload', reload)
  events.addEventListener('css', (e) => css(e.data))
})()</script>`

var idiomorph struct {
	once sync.Once
	src  string
	err  error
}

// idiomorphSource is the idiomorph midnight vendors
func idiomorphSource() (string, error) {
	idiomorph.once.Do(func() {
		l := lua.NewState()
		defer l.Close()
		if idiomorph.err = l.DoString(`return require('` + lua.MarkupMorphLibName + `')`); idiomorph.err != nil {
			return
		}
		idiomorph.src = l.ToString(-1)
		if idiomorph.src == "" {
			idiomorph.err = fmt.Errorf("dev: %s is empty", lua.MarkupMorphLibName)
		}
	})
	return idiomorph.src, idiomorph.err
}
