package restlua_test

import (
	"testing"
)

func TestLuaBase64(t *testing.T) {
	l := newLuaState(t)

	// Input "\xfb\xff\xfe\xff" produces "+//+/w==" in standard and "-__-_w==" in url-safe
	code := `
		local raw_bytes = "\251\255\254\255"

		-- Standard padded (default)
		local std = base64.encode(raw_bytes)
		assert(std == "+//+/w==", "expected +//+/w==, got " .. std)
		assert(base64.decode(std) == raw_bytes, "failed to decode std")

		-- Standard unpadded (raw = true / pad = false)
		local std_raw = base64.encode(raw_bytes, { raw = true })
		assert(std_raw == "+//+/w", "expected +//+/w, got " .. std_raw)
		assert(base64.encode(raw_bytes, { pad = false }) == "+//+/w", "expected pad=false to strip =")
		assert(base64.decode(std_raw) == raw_bytes, "failed to decode std_raw")

		-- URL-safe padded
		local url = base64.encode(raw_bytes, { url = true })
		assert(url == "-__-_w==", "expected -__-_w==, got " .. url)
		assert(base64.decode(url) == raw_bytes, "failed to auto-decode url")
		assert(base64.decode(url, { url = true }) == raw_bytes, "failed to decode url with opts")

		-- URL-safe unpadded
		local url_raw = base64.encode(raw_bytes, { url = true, raw = true })
		assert(url_raw == "-__-_w", "expected -__-_w, got " .. url_raw)
		assert(base64.encode(raw_bytes, { url = true, pad = false }) == "-__-_w", "expected url+pad=false")
		assert(base64.decode(url_raw) == raw_bytes, "failed to auto-decode url_raw")
	`
	if err := l.DoString(code); err != nil {
		t.Fatalf("lua execution failed: %v", err)
	}
}
