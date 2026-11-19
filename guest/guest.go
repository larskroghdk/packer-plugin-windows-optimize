// Package guest embeds the PowerShell scripts that the provisioner uploads and
// runs on the Windows guest.
package guest

import (
	"embed"
	"io/fs"
)

//go:embed scripts/*.ps1
var scripts embed.FS

const EntryScript = "Invoke-WindowsOptimize.ps1"

// Scripts returns every embedded guest script, keyed by file name.
//
// Returns:
//   - map[string][]byte: The script contents by file name.
//   - error: The embedded file system cannot be read.
//
// Author: Lars Krogh
// Version: 1.0.0
func Scripts() (map[string][]byte, error) {
	entries, err := fs.ReadDir(scripts, "scripts")
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		data, err := scripts.ReadFile("scripts/" + e.Name())
		if err != nil {
			return nil, err
		}
		out[e.Name()] = data
	}
	return out, nil
}
