package types

/* type Path struct {
	Post struct {
		Parameters []struct {
			Name     string `json:"name"`
			In       string `json:"in"`
			Required bool   `json:"required"`
			Style    string `json:"style"`
			Explode  bool   `json:"explode"`
			Schema   struct {
				Type string `json:"type"`
			} `json:"schema"`
			Example string `json:"example,omitempty"`
		} `json:"parameters"`
		Responses struct {
			Num200 struct {
				Description string `json:"description"`
			} `json:"200"`
		} `json:"responses"`
		Security []struct {
			Default []interface{} `json:"default"`
		} `json:"security"`
		XAuthType       string `json:"x-auth-type"`
		XThrottlingTier string `json:"x-throttling-tier"`
	} `json:"post"`
}

type OpenAPI struct {
	Version string `json:"openapi"`
	Info    struct {
		Title   string `json:"title"`
		Version string `json:"version"`
	} `json:"info"`
	Servers []struct {
		URL string `json:"url"`
	} `json:"servers"`
	Security []struct {
		Default []interface{} `json:"default"`
	} `json:"security"`

	Paths map[string]Path `json:"paths"`

	Components struct {
		SecuritySchemes struct {
			Default struct {
				Type  string `json:"type"`
				Flows struct {
					Implicit struct {
						AuthorizationURL string `json:"authorizationUrl"`
						Scopes           struct {
						} `json:"scopes"`
					} `json:"implicit"`
				} `json:"flows"`
			} `json:"default"`
		} `json:"securitySchemes"`
	} `json:"components"`
} */
