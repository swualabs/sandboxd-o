package main

import (
	"fmt"
	"os"

	"sandboxd-o/sandboxd-adm/cmd"
)

func main() {
	root := cmd.NewRoot()
	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
