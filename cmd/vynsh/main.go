package main

import (
	"os"

	"github.com/jaraslaukunin/vynsh/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
