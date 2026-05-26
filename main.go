package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/sergio-bershadsky/secondbrain-db/cmd"
)

type exitCoder interface{ ExitCode() int }

func main() {
	if err := cmd.Execute(); err != nil {
		var ec exitCoder
		if errors.As(err, &ec) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(ec.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
