package main

import (
	"flag"
	"fmt"
	"os"
)

var version = "dev"

func main() {
	flags := flag.NewFlagSet("plane-cli", flag.ExitOnError)
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "plane-cli is a work-in-progress Plane.so CLI.\n\nUsage:\n  plane-cli [--version]\n\n")
		flags.PrintDefaults()
	}

	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if *showVersion {
		fmt.Println(version)
		return
	}
	flags.Usage()
}
