module github.com/taybart/rest

go 1.17

replace github.com/taybart/args => ../../taybart/args

require (
	github.com/hashicorp/hcl/v2 v2.11.1
	github.com/taybart/args v0.0.0-20220224221651-d96033464fcd
	github.com/taybart/log v1.5.1
	github.com/zclconf/go-cty v1.8.0
)

require (
	github.com/agext/levenshtein v1.2.1 // indirect
	github.com/apparentlymart/go-textseg/v13 v13.0.0 // indirect
	github.com/google/go-cmp v0.3.1 // indirect
	github.com/mitchellh/go-wordwrap v0.0.0-20150314170334-ad45545899c7 // indirect
	golang.org/x/text v0.3.5 // indirect
)
