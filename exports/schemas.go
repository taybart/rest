package exports

import (
	"github.com/taybart/rest/request"
)

// componentSet turns the file's schemas blocks into named definitions as
// exports run into them, so a shape written as schemas.pet stays "pet"
// instead of being repeated everywhere. Only schemas that are used end up
// defined.
type componentSet struct {
	named map[string]any
	// ref writes a reference to a definition, a $ref for openapi
	ref func(def string) map[string]any

	defs  map[string]map[string]any
	order []string // definitions in the order they finished, deps first
	keys  map[string]string
}

func newComponentSet(named map[string]any, ref func(def string) map[string]any) *componentSet {
	return &componentSet{
		named: named,
		ref:   ref,
		defs:  map[string]map[string]any{},
		keys:  map[string]string{},
	}
}

// opts adds named schema lookups to schema options
func (c *componentSet) opts(base request.SchemaOptions) request.SchemaOptions {
	if c == nil || len(c.named) == 0 {
		return base
	}
	additional := base.Additional
	base.Ref = func(name string) map[string]any {
		return c.ref(c.define(name, additional))
	}
	return base
}

// define makes sure a definition exists for the schema and returns its name.
// expect { additional = true } loosens every object in the body, so a schema
// used both ways needs two definitions, the second gets a suffix.
func (c *componentSet) define(name string, additional bool) string {
	key := name + "|strict"
	if additional {
		key = name + "|additional"
	}
	if def, ok := c.keys[key]; ok {
		return def
	}
	def := name
	if _, taken := c.defs[def]; taken {
		def = name + "_strict"
		if additional {
			def = name + "_additional"
		}
	}
	c.keys[key] = def
	c.defs[def] = nil // reserved while its own body is built
	c.defs[def] = request.MatcherSchema(c.named[name], c.opts(request.SchemaOptions{Additional: additional}))
	c.order = append(c.order, def)
	return def
}
