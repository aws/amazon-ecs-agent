// +build tools

package tools

import (
	_ "github.com/fzipp/gocyclo"
	_ "github.com/golang/mock/mockgen"
	_ "golang.org/x/tools/cmd/cover"
	_ "golang.org/x/tools/cmd/goimports"
	_ "honnef.co/go/tools/cmd/staticcheck"
)