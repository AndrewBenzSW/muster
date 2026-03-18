package main

import (
	"os"

	"github.com/abenz1267/muster/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
