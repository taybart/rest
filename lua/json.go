package restlua

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"

	lua "github.com/taybart/midnight"
)

func registerJSONModule(l *lua.LState) {
	mod := l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"encode": jsonEncode,
		"decode": jsonDecode,
	})
	l.SetGlobal("json", mod)
}

func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// jsonObject is a JSON object that keeps its keys' order.
type jsonObject []jsonField

type jsonField struct {
	key   string
	value any
}

func (o jsonObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := marshalJSON(f.key)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := marshalJSON(f.value)
		if err != nil {
			return nil, err
		}
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// EncodeJSON encodes a lua value like json.encode (objects keep their keys'
// order, no HTML escaping).
func EncodeJSON(v lua.LValue) ([]byte, error) {
	goVal, err := luaToJSONValue(v, make(map[*lua.LTable]bool))
	if err != nil {
		return nil, err
	}
	return marshalJSON(goVal)
}

func luaToJSONValue(val lua.LValue, visited map[*lua.LTable]bool) (any, error) {
	switch v := val.(type) {
	case *lua.LNilType:
		return nil, nil
	case lua.LBool:
		return bool(v), nil
	case lua.LString:
		return string(v), nil
	case lua.LNumber:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("unexpected number value '%v'", v)
		}
		return f, nil
	case *lua.LTable:
		if visited[v] {
			return nil, fmt.Errorf("circular reference")
		}
		visited[v] = true
		defer delete(visited, v)

		maxn := v.MaxN()
		count := 0
		hasNum := false
		hasStr := false
		invalidKey := false
		isSeq := true

		v.ForEach(func(key, _ lua.LValue) {
			count++
			switch k := key.(type) {
			case lua.LNumber:
				hasNum = true
				idx := int(k)
				if float64(idx) != float64(k) || idx < 1 || idx > maxn {
					isSeq = false
				}
			case lua.LString:
				hasStr = true
				isSeq = false
			default:
				invalidKey = true
				isSeq = false
			}
		})

		if invalidKey || (hasNum && hasStr) {
			return nil, fmt.Errorf("invalid table: mixed or invalid key types")
		}
		if hasNum && (!isSeq || count != maxn) {
			return nil, fmt.Errorf("invalid table: sparse or non-sequential array")
		}
		if count == 0 || isSeq {
			arr := make([]any, maxn)
			for i := 1; i <= maxn; i++ {
				elem, err := luaToJSONValue(v.RawGetInt(i), visited)
				if err != nil {
					return nil, err
				}
				arr[i-1] = elem
			}
			return arr, nil
		}

		// (in the order the keys were added, like JSON.stringify)
		obj := make(jsonObject, 0, count)
		for key, value := v.Next(lua.LNil); key != lua.LNil; key, value = v.Next(key) {
			elem, err := luaToJSONValue(value, visited)
			if err != nil {
				return nil, err
			}
			obj = append(obj, jsonField{string(key.(lua.LString)), elem})
		}
		return obj, nil
	default:
		return nil, fmt.Errorf("unexpected type '%s'", val.Type().String())
	}
}

func jsonEncode(l *lua.LState) int {
	val := l.Get(1)
	goVal, err := luaToJSONValue(val, make(map[*lua.LTable]bool))
	if err != nil {
		l.RaiseError("json.encode: %v", err)
		return 0
	}
	b, err := marshalJSON(goVal)
	if err != nil {
		l.RaiseError("json.encode: %v", err)
		return 0
	}
	l.Push(lua.LString(b))
	return 1
}

func jsonDecode(l *lua.LState) int {
	arg := l.Get(1)
	str, ok := arg.(lua.LString)
	if !ok {
		l.RaiseError("json.decode: expected argument of type string, got %s", arg.Type().String())
		return 0
	}
	if str == "" {
		l.RaiseError("json.decode: cannot decode empty string")
		return 0
	}
	var out any
	if err := json.Unmarshal([]byte(str), &out); err != nil {
		l.RaiseError("json.decode: %v", err)
		return 0
	}
	l.Push(AnyToLValue(l, out))
	return 1
}
