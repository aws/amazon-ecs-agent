package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kubernetes-csi/csi-proxy/v2/pkg/utils"
)

// Implements the Filesystem OS API calls. All code here should be very simple
// pass-through to the OS APIs. Any logic around the APIs should go in
// pkg/filesystem/filesystem.go so that logic can be easily unit-tested
// without requiring specific OS environments.

// HostAPI is the exposed Filesystem API
type HostAPI interface {
	PathExists(path string) (bool, error)
	PathValid(path string) (bool, error)
	Mkdir(path string) error
	Rmdir(path string, force bool) error
	RmdirContents(path string) error
	CreateSymlink(oldname string, newname string) error
	IsSymlink(path string) (bool, error)
}

type filesystemAPI struct{}

// check that filesystemAPI implements HostAPI
var _ HostAPI = &filesystemAPI{}

func New() HostAPI {
	return filesystemAPI{}
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (filesystemAPI) PathExists(path string) (bool, error) {
	return pathExists(path)
}

func pathValid(path string) (bool, error) {
	cmd := `Test-Path $Env:remotepath`
	cmdEnv := fmt.Sprintf("remotepath=%s", path)
	output, err := utils.RunPowershellCmd(cmd, cmdEnv)
	if err != nil {
		return false, fmt.Errorf("returned output: %s, error: %v", string(output), err)
	}

	return strings.HasPrefix(strings.ToLower(string(output)), "true"), nil
}

// PathValid determines whether all elements of a path exist
//
//	https://docs.microsoft.com/en-us/powershell/module/microsoft.powershell.management/test-path?view=powershell-7
//
// for a remote path, determines whether connection is ok
//
//	e.g. in a SMB server connection, if password is changed, connection will be lost, this func will return false
func (filesystemAPI) PathValid(path string) (bool, error) {
	return pathValid(path)
}

// Mkdir makes a dir with `os.MkdirAll`.
func (filesystemAPI) Mkdir(path string) error {
	return os.MkdirAll(path, 0755)
}

// Rmdir removes a dir with `os.Remove`, if force is true then `os.RemoveAll` is used instead.
func (filesystemAPI) Rmdir(path string, force bool) error {
	if force {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

// RmdirContents removes the contents of a directory with `os.RemoveAll`
func (filesystemAPI) RmdirContents(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()

	files, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, file := range files {
		candidatePath := filepath.Join(path, file)
		err = os.RemoveAll(candidatePath)
		if err != nil {
			return err
		}
	}

	return nil
}

// CreateSymlink creates newname as a symbolic link to oldname.
func (filesystemAPI) CreateSymlink(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}

// IsSymlink - returns true if tgt is a mount point.
// A path is considered a mount point if:
//   - directory exists and
//   - it is a soft link and
//   - the target path of the link exists.
//
// If tgt path does not exist, it returns an error
// if tgt path exists, but the source path tgt points to does not exist, it returns false without error.
func (filesystemAPI) IsSymlink(tgt string) (bool, error) {
	// This code is similar to k8s.io/kubernetes/pkg/util/mount except the pathExists usage.
	// Also in a remote call environment the os error cannot be passed directly back, hence the callers
	// are expected to perform the isExists check before calling this call in CSI proxy.
	stat, err := os.Lstat(tgt)
	if err != nil {
		return false, err
	}

	// If its a link and it points to an existing file then its a mount point.
	if stat.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(tgt)
		if err != nil {
			return false, fmt.Errorf("readlink error: %v", err)
		}
		exists, err := pathExists(target)
		if err != nil {
			return false, err
		}
		return exists, nil
	}

	return false, nil
}
