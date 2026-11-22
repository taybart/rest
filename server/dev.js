(() => {
  let connected = false
  const errorHeader = '__ERROR_HEADER__'
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
    root.innerHTML = `<style>
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
          <div class="sleepy">` + moon(56) + `<i class="z">z</i><i class="z">z</i><i class="z">z</i></div>
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
    <div class="down" hidden>` + moon(22) + `<div>rest is asleep, waiting for it to wake up <span>(see its output)</span></div></div>`
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
    s.src = '__MORPH_PATH__'
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
      if (__MARKUP_MOUNTED__) return hard('interactive page (its lua state is per page load)')
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
  const events = new EventSource('__EVENTS_PATH__')
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
})()
