package guest

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// TestScriptsAreComplete checks that the entry script is embedded, that every
// part it dot-sources is embedded, and that every embedded part is loaded.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestScriptsAreComplete(t *testing.T) {
	scripts, err := Scripts()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := scripts[EntryScript]
	if !ok {
		t.Fatalf("%s is not embedded", EntryScript)
	}
	m := regexp.MustCompile(`foreach \(\$part in ([^)]+)\)`).FindSubmatch(entry)
	if m == nil {
		t.Fatal("the entry script's part list was not found")
	}
	loaded := map[string]bool{EntryScript: true}
	for _, part := range strings.Split(string(m[1]), ",") {
		name := "WindowsOptimize." + strings.Trim(strings.TrimSpace(part), "'") + ".ps1"
		loaded[name] = true
		if _, ok := scripts[name]; !ok {
			t.Errorf("the entry script loads %s, which is not embedded", name)
		}
	}
	for name := range scripts {
		if !loaded[name] {
			t.Errorf("%s is embedded but never loaded", name)
		}
	}
}

// TestScriptsAreWindowsPowerShellSafe checks the properties Windows PowerShell
// 5.1 needs: CRLF line endings, no byte order mark issues from non-ASCII text,
// and the version requirement on the first line.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestScriptsAreWindowsPowerShellSafe(t *testing.T) {
	scripts, err := Scripts()
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range scripts {
		if !bytes.HasPrefix(data, []byte("#Requires -Version 5.1\r\n")) {
			t.Errorf("%s must start with #Requires -Version 5.1 and use CRLF", name)
		}
		if bytes.Count(data, []byte("\n")) != bytes.Count(data, []byte("\r\n")) {
			t.Errorf("%s has LF-only line endings", name)
		}
		for i, b := range data {
			if b > 0x7e || (b < 0x20 && b != '\r' && b != '\n') {
				t.Errorf("%s: byte %d is 0x%02x; Windows PowerShell 5.1 reads BOM-less files as ANSI, keep scripts ASCII", name, i, b)
				break
			}
		}
	}
}
