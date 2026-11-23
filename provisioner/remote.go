package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/larskroghdk/packer-plugin-windows-optimize/guest"
)

type remote struct {
	comm packer.Communicator
	ui   packer.Ui
	dir  string
	seq  int
}

// newRemote prepares guest access for one run. The working directory below
// C:\Windows\Temp is unique to runID.
//
// Author: Lars Krogh
// Version: 1.0.0
func newRemote(comm packer.Communicator, ui packer.Ui, runID string) *remote {
	return &remote{comm: comm, ui: ui, dir: `C:\Windows\Temp\packer-windows-optimize-` + runID}
}

// path returns the guest path of name in the working directory.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *remote) path(name string) string { return r.dir + `\` + name }

// powershellCommand returns a non-interactive Windows PowerShell command line
// with args appended.
//
// Author: Lars Krogh
// Version: 1.0.0
func powershellCommand(args string) string {
	return `powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass ` + args
}

// install creates the private working directory and uploads every guest
// script.
//
// Returns:
//   - error: The directory cannot be created or a script cannot be uploaded.
//
// Author: Lars Krogh
// Version: 1.1.0
func (r *remote) install(ctx context.Context) error {
	create := powershellCommand(`-Command "` + privateDirectoryScript(r.dir) + `"`)
	if err := r.exec(ctx, create); err != nil {
		return fmt.Errorf("create guest working directory %s: %w", r.dir, err)
	}
	scripts, err := guest.Scripts()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(scripts))
	for name := range scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := r.comm.Upload(r.path(name), bytes.NewReader(scripts[name]), nil); err != nil {
			return fmt.Errorf("upload %s: %w", name, err)
		}
	}
	return nil
}

// privateDirectoryScript returns PowerShell that creates dir with an ACL
// granting access only to SYSTEM and Administrators. The directory and its ACL
// are created in one call, and an existing directory is accepted only when
// SYSTEM, Administrators or the current user owns it. A non-administrator
// cannot give a directory those owners, so one created in advance to tamper
// with the uploaded scripts is rejected. The ACL is then reapplied. Only the
// access rules are written, which needs no SeSecurityPrivilege.
//
// Author: Lars Krogh
// Version: 1.0.0
func privateDirectoryScript(dir string) string {
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'; $d = '%s'; $acl = New-Object System.Security.AccessControl.DirectorySecurity; $acl.SetAccessRuleProtection($true, $false); foreach ($id in 'S-1-5-18', 'S-1-5-32-544') { $rule = New-Object System.Security.AccessControl.FileSystemAccessRule(([System.Security.Principal.SecurityIdentifier]$id), 'FullControl', 'ContainerInherit, ObjectInherit', 'None', 'Allow'); $acl.AddAccessRule($rule) }; $null = [IO.Directory]::CreateDirectory($d, $acl); $owner = (Get-Acl -LiteralPath $d).GetOwner([System.Security.Principal.SecurityIdentifier]).Value; if ($owner -notin 'S-1-5-18', 'S-1-5-32-544', [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value) { throw ('Guest working directory ' + $d + ' is owned by ' + $owner) }; (New-Object System.IO.DirectoryInfo($d)).SetAccessControl($acl)`, strings.ReplaceAll(dir, "'", "''"))
}

// cleanup removes the working directory and everything in it.
//
// Returns:
//   - error: The command failed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *remote) cleanup(ctx context.Context) error {
	cmd := powershellCommand(fmt.Sprintf(`-Command "$ErrorActionPreference = 'Stop'; Remove-Item -LiteralPath '%s' -Recurse -Force"`, r.dir))
	return r.exec(ctx, cmd)
}

// exec runs command on the guest, streaming its output to the Packer UI.
//
// Returns:
//   - error: The command could not be run or exited with a non-zero status.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *remote) exec(ctx context.Context, command string) error {
	cmd := &packer.RemoteCmd{Command: command}
	if err := cmd.RunWithUi(ctx, r.comm, r.ui); err != nil {
		return err
	}
	if code := cmd.ExitStatus(); code != 0 {
		return fmt.Errorf("command exited with status %d", code)
	}
	return nil
}

// action runs one guest action. input, when not nil, is uploaded as JSON and
// passed as -InputPath. Each call uses new numbered file names, so results of
// earlier actions are kept for troubleshooting until cleanup.
//
// Returns:
//   - []byte: The JSON result, without a byte order mark.
//   - error: The upload, the action or the download failed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *remote) action(ctx context.Context, name string, input any) ([]byte, error) {
	r.seq++
	prefix := fmt.Sprintf("%02d-%s", r.seq, strings.ToLower(name))
	args := fmt.Sprintf(`-File "%s" -Action %s -OutputPath "%s"`, r.path(guest.EntryScript), name, r.path(prefix+"-output.json"))
	if input != nil {
		inputPath, err := r.upload(prefix+"-input.json", input)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		args += fmt.Sprintf(` -InputPath "%s"`, inputPath)
	}
	if err := r.exec(ctx, powershellCommand(args)); err != nil {
		return nil, fmt.Errorf("guest action %s failed: %w", name, err)
	}
	var out bytes.Buffer
	if err := r.comm.Download(r.path(prefix+"-output.json"), &out); err != nil {
		return nil, fmt.Errorf("%s: download result: %w", name, err)
	}
	return bytes.TrimPrefix(out.Bytes(), []byte("\xef\xbb\xbf")), nil
}

// upload writes v as JSON to name in the working directory.
//
// Returns:
//   - string: The guest path of the file.
//   - error: v cannot be encoded or the upload failed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *remote) upload(name string, v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	remotePath := r.path(name)
	if err := r.comm.Upload(remotePath, bytes.NewReader(data), nil); err != nil {
		return "", fmt.Errorf("upload %s: %w", name, err)
	}
	return remotePath, nil
}
