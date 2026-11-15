package server

import (
	"bufio"
	"fmt"
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
//   - handler `cache` is off (request block caches stay on)
//   - html rest renders gets a script that reloads the page when rest says
//     so (over /_rest/dev), and after rest restarts
//   - under `run -w` (w/ RUN_EVENTS_FD), rest handles file changes itself:
//     a change to the rest file, an import, or a file read() read needs a
//     new parse, so rest asks run to restart it. A .css file under
//     `directory` is swapped in place, and anything else (pages, markdown,
//     scripts, ...) reloads the page: re-rendered and morphed in, so scroll
//     and what's typed survive (interactive pages reload, their lua state is
//     per page load).

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
// morphed in (keeping scroll, focus, and inputs), interactive ones (w/ the
// midnight client) reload, since their lua state is per page load. "css
// <path>" swaps that stylesheet. When rest comes back after a restart, the
// event stream reconnects, and the page reloads.
var devScript = `<script data-rest-dev>(() => {
  let connected = false
  const morphLib = () => window.Idiomorph ? Promise.resolve() : new Promise((ok, fail) => {
    const s = document.createElement('script')
    s.src = '` + devMorphPath + `'
    s.onload = ok
    s.onerror = fail
    document.head.append(s)
  })
  const reload = async () => {
    try {
      if (window.midnight) return location.reload()
      const res = await fetch(location.href)
      if (!res.ok) return location.reload()
      const next = new DOMParser().parseFromString(await res.text(), 'text/html')
      await morphLib()
      document.title = next.title
      Idiomorph.morph(document.body, next.body, { ignoreActiveValue: true })
    } catch (e) {
      console.error('rest dev:', e)
      location.reload()
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
  events.onopen = () => {
    if (connected) location.reload() // (rest restarted)
    connected = true
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
