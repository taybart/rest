package request

type Config struct {
	NoFollowRedirect    bool   `hcl:"no_follow_redirect,optional"`
	NoCookies           bool   `hcl:"no_cookies,optional"`
	UserAgent           string `hcl:"user_agent,optional"`
	InsecureNoVerifyTLS bool   `hcl:"insecure_no_verify_tls,optional"`
	NamespaceImports    bool   `hcl:"namespace_imports,optional"`
	SkipImported        bool   `hcl:"skip_imported,optional"`
	NoFlags             bool   `hcl:"no_flags,optional"`
	DefaultContentType  bool   `hcl:"default_content_type,optional"`
	// language for scripts (after, cli, server), "lua" or "erde"
	// unset uses heredoc hints (<<LUA, <<ERDE), then lua
	Lang string `hcl:"lang,optional"`
}

func DefaultConfig() Config {
	return Config{
		NoFollowRedirect:    false,
		NoCookies:           false,
		UserAgent:           "rest-client/2.0",
		InsecureNoVerifyTLS: false,
		NamespaceImports:    true,
		SkipImported:        false,
		NoFlags:             false,
		DefaultContentType:  true,
	}
}
