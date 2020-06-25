package mocks

import (
	"os"

	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
)

type MockFile struct {
	ChmodImpl func(os.FileMode) error
	NameImpl  func() string
	SyncImpl  func() error
	WriteImpl func([]byte) (int, error)
}

func NewMockFile() oswrapper.File {
	return &MockFile{
		ChmodImpl: func(os.FileMode) error {
			return nil
		},
		NameImpl: func() string {
			return ""
		},
		SyncImpl: func() error {
			return nil
		},
		WriteImpl: func(bytes []byte) (i int, e error) {
			return 0, nil
		},
	}
}

func (f *MockFile) Name() string {
	return f.NameImpl()
}

func (f *MockFile) Close() error {
	return nil
}

func (f *MockFile) Chmod(fileMode os.FileMode) error {
	return f.ChmodImpl(fileMode)
}

func (f *MockFile) Write(content []byte) (int, error) {
	return f.WriteImpl(content)
}

func (f *MockFile) WriteAt(b []byte, off int64) (n int, err error) {
	return 0, nil
}

func (f *MockFile) Sync() error {
	return f.SyncImpl()
}

func (f *MockFile) Read([]byte) (int, error) {
	return 0, nil
}
