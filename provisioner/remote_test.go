package provisioner

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestPrivateDirectoryScript runs the working directory script under Windows
// PowerShell twice and checks that the directory admits only SYSTEM and
// Administrators, and that a directory the current user owns is accepted on
// the second run. PSModulePath is cleared so that a PowerShell 7 parent shell
// cannot load its modules into Windows PowerShell.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestPrivateDirectoryScript(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires Windows PowerShell")
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("powershell.exe is not available")
	}
	run := func(script string) string {
		t.Helper()
		cmd := exec.Command(powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
		for _, kv := range os.Environ() {
			if !strings.HasPrefix(strings.ToUpper(kv), "PSMODULEPATH=") {
				cmd.Env = append(cmd.Env, kv)
			}
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	dir := filepath.Join(t.TempDir(), "work dir's")
	quoted := "'" + strings.ReplaceAll(dir, "'", "''") + "'"
	t.Cleanup(func() {
		run(`$p = ` + quoted + `; $a = Get-Acl -LiteralPath $p; $a.AddAccessRule((New-Object System.Security.AccessControl.FileSystemAccessRule([System.Security.Principal.WindowsIdentity]::GetCurrent().User, 'FullControl', 'ContainerInherit, ObjectInherit', 'None', 'Allow'))); (New-Object System.IO.DirectoryInfo($p)).SetAccessControl($a)`)
	})
	run(privateDirectoryScript(dir))
	run(privateDirectoryScript(dir))
	got := run(`$a = Get-Acl -LiteralPath ` + quoted + `; [string]$a.AreAccessRulesProtected + ' ' + ((@($a.Access | ForEach-Object { $_.IdentityReference.Translate([System.Security.Principal.SecurityIdentifier]).Value + '=' + $_.FileSystemRights }) | Sort-Object) -join ',')`)
	if want := "True S-1-5-18=FullControl,S-1-5-32-544=FullControl"; got != want {
		t.Fatalf("ACL = %q, want %q", got, want)
	}
}
