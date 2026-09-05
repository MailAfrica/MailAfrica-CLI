package main

import (
	"fmt"
	"os"

	"github.com/MailAfrica/MailAfrica-CLI/internal/cmd"
)

func main() {
	if err := cmd.NewRootCmd(os.Args[1:]).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
