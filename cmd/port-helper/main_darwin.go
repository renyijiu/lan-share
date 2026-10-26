package main

import (
	"flag"
	"fmt"
	"github.com/renyijiu/lan-share/internal/portbind"
	"os"
)

func main() {
	socket := flag.String("socket", "", "private control socket")
	uid := flag.Int("uid", 0, "recipient uid")
	flag.Parse()
	if flag.NArg() != 0 {
		os.Exit(2)
	}
	if err := portbind.RunHelper(*socket, *uid); err != nil {
		fmt.Fprintln(os.Stderr, "Port handoff failed:", err)
		os.Exit(1)
	}
}
