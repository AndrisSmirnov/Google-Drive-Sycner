// Command syncer copies selected files and folders of Google Drive to a
// local drive. Without arguments it opens the menu; "syncer sync" runs a
// sync without it.
package main

import (
	"os"

	"syncer-gdrive/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
