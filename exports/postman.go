package exports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/taybart/rest/exports/postman"
	"github.com/taybart/rest/exports/templates"
)

// ToPostman writes requests as a postman v2.1 collection named after the
// rest file
func ToPostman(w io.Writer, filename string, reqs []templates.Request) error {
	name := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	c := postman.CreateCollection(name, "")
	for _, r := range reqs {
		item, err := postmanItem(r)
		if err != nil {
			return fmt.Errorf("%s: %w", r.Label, err)
		}
		c.AddItem(item)
	}
	if err := c.Write(w, postman.V210); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func postmanItem(r templates.Request) (*postman.Items, error) {
	u, err := postmanURL(r)
	if err != nil {
		return nil, err
	}
	req := &postman.Request{
		URL:    u,
		Method: postman.Method(r.Method),
	}

	// Authorization goes in auth below so it shows up in postman's auth tab
	headers := map[string]string{}
	for k, v := range r.RequestHeaders {
		headers[k] = v
	}
	if r.UserAgent != "" {
		headers["User-Agent"] = r.UserAgent
	}
	if c := r.CookieHeader(); c != "" {
		headers["Cookie"] = c
	}
	for _, k := range sortedKeys(headers) {
		req.Header = append(req.Header, &postman.Header{Key: k, Value: headers[k]})
	}

	switch r.Auth.Type {
	case "basic":
		req.Auth = postman.CreateAuth(postman.Basic,
			postman.CreateAuthParam("username", r.Auth.User),
			postman.CreateAuthParam("password", r.Auth.Password),
		)
	case "bearer":
		req.Auth = postman.CreateAuth(postman.Bearer,
			postman.CreateAuthParam("token", r.Auth.Token),
		)
	}

	if r.Body != "" {
		req.Body = &postman.Body{Mode: "raw", Raw: r.Body}
		if r.IsJSON() {
			// pretty so it reads well in postman's body editor
			var buf bytes.Buffer
			if json.Indent(&buf, []byte(r.Body), "", "  ") == nil {
				req.Body.Raw = buf.String()
			}
			req.Body.Options = &postman.BodyOptions{Raw: postman.BodyOptionsRaw{Language: postman.JSON}}
		}
	}

	item := &postman.Items{Name: r.Label, Request: req}
	if r.Expect.Status != 0 {
		item.Events = append(item.Events, postman.CreateEvent(postman.Test, []string{
			fmt.Sprintf("pm.test(%q, function () {", fmt.Sprintf("status is %d", r.Expect.Status)),
			fmt.Sprintf("  pm.response.to.have.status(%d);", r.Expect.Status),
			"});",
		}))
	}
	return item, nil
}

func postmanURL(r templates.Request) (*postman.URL, error) {
	parsed, err := url.Parse(r.BaseURL)
	if err != nil {
		return nil, err
	}
	u := &postman.URL{
		Raw:      r.URL(),
		Protocol: parsed.Scheme,
		Host:     strings.Split(parsed.Hostname(), "."),
		Port:     parsed.Port(),
	}
	// "/a/b" -> ["a", "b"], postman joins path segments with "/" itself
	if p := strings.TrimPrefix(parsed.Path, "/"); p != "" {
		u.Path = strings.Split(p, "/")
	}
	for _, k := range sortedKeys(r.Query) {
		for _, v := range r.Query[k] {
			u.Query = append(u.Query, &postman.QueryParam{Key: k, Value: v})
		}
	}
	return u, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
