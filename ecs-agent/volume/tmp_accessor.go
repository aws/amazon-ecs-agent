package volume

import (
	"os"
	"path/filepath"
)

// tmpAccessor is used for testing purposes only. It implements the
// volume accessor interface to access/write files from/to the temp directory.
type tmpAccessor struct {
	// tmpDir denotes a subdirectory under /tmp which requires to be accessed.
	tmpDir string
}

func NewTmpAccessor(tmpDirName string) VolumeAccessor {
	return &tmpAccessor{
		tmpDir: tmpDirName,
	}
}

// CopyToVolume copies the source file into a subdirectory under /tmp.
// Name of the subdirectory can be configured by specifying a value
// for tmpDir while creating tmpAccessor.
func (t *tmpAccessor) CopyToVolume(src, dst string, mode os.FileMode) error {
	// Read source file.
	contents, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// Create destination directory.
	finalDstDir := filepath.Join("/tmp", t.tmpDir)
	err = os.MkdirAll(finalDstDir, mode)
	if err != nil {
		return err
	}

	finalDst := filepath.Join(finalDstDir, dst)
	return os.WriteFile(finalDst, contents, mode)
}

func (t *tmpAccessor) DeleteAll(string) error {
	return nil
}
