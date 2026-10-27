package restlua_test

import (
	"testing"

	restlua "github.com/taybart/rest/lua"
	lua "github.com/yuin/gopher-lua"
)

func TestJSONAndUUIDModules(t *testing.T) {
	l := lua.NewState()
	defer l.Close()

	if err := restlua.RegisterModules(l); err != nil {
		t.Fatalf("RegisterModules failed: %v", err)
	}

	script := `
		-- encode object and decode back
		local encoded = json.encode({ name = "alice", count = 3, active = true, url = "https://example.com?a=1&b=2" })
		assert(encoded:find("&") ~= nil, "expected unescaped ampersand in json.encode")
		local decoded = json.decode(encoded)
		assert(decoded.name == "alice", "expected name alice")
		assert(decoded.count == 3, "expected count 3")
		assert(decoded.active == true, "expected active true")

		-- encode array and empty table
		assert(json.encode({ "a", "b" }) == '["a","b"]', "expected array encoding")
		assert(json.encode({}) == '[]', "expected empty table to encode as []")
		assert(json.encode(nil) == 'null', "expected nil to encode as null")

		-- error cases via pcall
		local ok1 = pcall(json.decode, "")
		assert(not ok1, "expected empty string decode to fail")
		local ok2 = pcall(json.decode, "{bad}")
		assert(not ok2, "expected invalid json decode to fail")

		-- uuid module
		local id = uuid.newv4()
		assert(uuid.is_uuid(id), "expected valid uuid")
		assert(uuid.is_uuidv4(id), "expected valid uuidv4")
		assert(not uuid.is_uuid("not-a-uuid"), "expected invalid uuid to return false")
		assert(not uuid.is_uuidv4("not-a-uuid"), "expected invalid uuidv4 to return false")

		-- ulid module (both ulid.new() and ulid())
		local u1 = ulid.new()
		local u2 = ulid(1700000000000)
		assert(#u1 == 26, "expected 26-char ulid")
		assert(ulid.is_ulid(u1), "expected valid ulid")
		assert(ulid.is_ulid(u2), "expected valid ulid with custom timestamp")
		assert(ulid.timestamp(u2) == 1700000000000, "expected matching ulid timestamp")
		assert(not ulid.is_ulid("not-a-ulid"), "expected invalid ulid to return false")

		-- nanoid module (both nanoid.new() and nanoid())
		local n1 = nanoid.new()
		local n2 = nanoid(10)
		local n3 = nanoid.new(8, "abc")
		assert(#n1 == 21 and nanoid.is_nanoid(n1), "expected 21-char nanoid")
		assert(#n2 == 10 and nanoid.is_nanoid(n2, 10), "expected 10-char nanoid")
		assert(#n3 == 8 and n3:match("^[abc]+$") ~= nil, "expected custom alphabet nanoid")
		assert(not nanoid.is_nanoid("too-short"), "expected invalid nanoid to return false")
	`
	if err := l.DoString(script); err != nil {
		t.Fatalf("lua script failed: %v", err)
	}
}
