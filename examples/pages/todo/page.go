package todo

import (
	"net/http"
	"path/filepath"

	"github.com/taybart/rest/pages"
)

// Styles serves tb.css (the page's base styles, next to it in uiDir). The page
// links it relatively ("tb.css"), so it's served next to the page's route.
func Styles(uiDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(uiDir, "tb.css"))
	})
}

// Page is the todo page (ui/app.mn, in uiDir), w/ the store's methods as
// the `app.todos` module. router is shown on the page.
func Page(eng *pages.Engine, store *Store, uiDir, router string) http.Handler {
	return eng.File(filepath.Join(uiDir, "app.mn"),
		pages.Props(func(r *http.Request) (any, error) {
			return map[string]any{"router": router}, nil
		}),
		// plain Go methods: the context is the request being handled, args
		// and results are converted (Todo becomes a table by its json tags),
		// and errors are lua errors
		pages.Funcs("app.todos", map[string]any{
			"list":   store.List,
			"add":    store.Add,
			"toggle": store.Toggle,
			"remove": store.Remove,
		}),
	)
}
