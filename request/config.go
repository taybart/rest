package request

type Config struct {
	NoFollowRedirect    bool   `hcl:"no_follow_redirect,optional"`
	NoCookies           bool   `hcl:"no_cookies,optional"`
	UserAgent           string `hcl:"user_agent,optional"`
	InsecureNoVerifyTLS bool   `hcl:"insecure_no_verify_tls,optional"`
}

func DefaultConfig() Config {
	return Config{
		NoFollowRedirect:    false,
		NoCookies:           false,
		UserAgent:           "rest-client/2.0",
		InsecureNoVerifyTLS: false,
	}
}
