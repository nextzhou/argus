package main

import (
	"os"
)

var version = "dev"

func main() {
	os.Stdout.WriteString("argus version " + version + "\n")
}
