package main

import (
	"os"

	"github.com/sergio-bershadsky/secondbrain-db/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
