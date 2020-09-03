// +build tools

package main

import (
	_ "golang.org/x/tools/cmd/cover"
	_ "github.com/golang/mock/mockgen"
	_ "golang.org/x/tools/cmd/goimports"
	_ "github.com/fzipp/gocyclo"
	_ "honnef.co/go/tools/cmd/staticcheck"
)

