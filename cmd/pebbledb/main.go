package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	code := 0
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		var exitErr *exitCodeError
		if errors.As(err, &exitErr) {
			code = exitErr.code
		} else {
			code = 1
		}
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	os.Exit(code)
}
