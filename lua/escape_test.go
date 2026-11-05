package restlua_test

import (
	"strings"
	"testing"
)

func TestLuaHTML(t *testing.T) {
	l := newLuaState(t)
	code := `
		-- escape
		local e = html.escape([[<a href="x">Tom & Jerry's</a>]])
		assert(e == "&lt;a href=&#34;x&#34;&gt;Tom &amp; Jerry&#39;s&lt;/a&gt;", "escape: " .. e)

		-- unescape named, decimal and hex entities
		local u = html.unescape("it&#39;s &quot;fine&quot; &amp; &lt;ok&gt; &#x27;x&#x27; caf&eacute;&hellip;")
		assert(u == [[it's "fine" & <ok> 'x' café…]], "unescape: " .. u)

		-- no double-decoding
		assert(html.unescape("&amp;quot;") == "&quot;", "double decode")

		-- round trip
		local s = [[<script>alert('x' && "y")</script>]]
		assert(html.unescape(html.escape(s)) == s, "round trip")
	`
	if err := l.DoString(code); err != nil {
		t.Fatalf("lua execution failed: %v", err)
	}
}

func TestLuaURL(t *testing.T) {
	l := newLuaState(t)
	code := `
		-- query escaping: space -> +
		local q = url.escape("a b&c=d/é")
		assert(q == "a+b%26c%3Dd%2F%C3%A9", "escape: " .. q)
		assert(url.unescape(q) == "a b&c=d/é", "unescape")
		assert(url.unescape("a%20b+c") == "a b c", "unescape %20 and +")

		-- path escaping: space -> %20, + kept literal on decode
		local p = url.path_escape("a b/c+d")
		assert(p == "a%20b%2Fc+d", "path_escape: " .. p)
		assert(url.path_unescape(p) == "a b/c+d", "path_unescape")
		assert(url.path_unescape("a+b") == "a+b", "path_unescape keeps +")
	`
	if err := l.DoString(code); err != nil {
		t.Fatalf("lua execution failed: %v", err)
	}
}

func TestLuaURLUnescapeError(t *testing.T) {
	l := newLuaState(t)
	for _, fn := range []string{"url.unescape", "url.path_unescape"} {
		err := l.DoString(fn + `("%zz")`)
		if err == nil || !strings.Contains(err.Error(), fn) {
			t.Fatalf("%s: expected error mentioning %s, got %v", fn, fn, err)
		}
	}
}
