// Command yanki syncs a directory of Markdown notes to Anki through
// AnkiConnect.
package main

import (
	"os"

	"github.com/ihaaae/yanki/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
