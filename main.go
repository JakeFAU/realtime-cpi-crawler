// The main package for the webcrawler executable.
package main

import (
	"github.com/JakeFAU/realtime-cpi/webcrawler/cmd"
)

// main is the entry point of the application.
// It defers all execution to the Cobra CLI library.
func main() {
	cmd.Execute()
}
