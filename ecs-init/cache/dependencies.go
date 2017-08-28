// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package cache

// This file exists to capture the dependencies of the cache package.
// Dependencies include other packages with struct-level functions as well as
// package-level functions.  These interfaces are then used to create mocks
// for the unit tests.

//go:generate mockgen.sh $GOPACKAGE $GOFILE

import (
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// httpGetter captures the only method used from the net/http package
type httpGetter interface {
	Get(url string) (resp *http.Response, err error)
}

// fileSystem captures related functions from os, io, and io/ioutil packages
type fileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	TempFile(dir, prefix string) (f *os.File, err error)
	Remove(path string)
	TeeReader(r io.Reader, w io.Writer) io.Reader
	Copy(dst io.Writer, src io.Reader) (written int64, err error)
	Rename(oldpath, newpath string) error
	ReadAll(r io.Reader) ([]byte, error)
	Open(name string) (file io.ReadCloser, err error)
	Stat(name string) (fileinfo fileSizeInfo, err error)
	Base(path string) string
	WriteFile(filename string, data []byte, perm os.FileMode) error
}

type fileSizeInfo interface {
	Size() int64
}

type instanceMetadata interface {
	Region() (string, error)
}

// standardFS delegates to the package-level functions
type standardFS struct{}

func (s *standardFS) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (s *standardFS) TempFile(dir, prefix string) (*os.File, error) {
	return ioutil.TempFile(dir, prefix)
}

func (s *standardFS) Remove(path string) {
	os.Remove(path)
}

func (s *standardFS) TeeReader(r io.Reader, w io.Writer) io.Reader {
	return io.TeeReader(r, w)
}

func (s *standardFS) Copy(dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, src)
}

func (s *standardFS) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (s *standardFS) ReadAll(r io.Reader) ([]byte, error) {
	return ioutil.ReadAll(r)
}

func (s *standardFS) Open(name string) (io.ReadCloser, error) {
	return os.Open(name)
}

func (s *standardFS) Stat(name string) (fileSizeInfo, error) {
	return os.Stat(name)
}

func (s *standardFS) Base(path string) string {
	return filepath.Base(path)
}

func (s *standardFS) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return ioutil.WriteFile(filename, data, perm)
}

// customGetter is very similar to http.DefaultClient, but sets a shorter
// timeout
type _customGetter struct {
	client *http.Client
}

var customGetter = &_customGetter{
	client: &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		}},
}

func (c *_customGetter) Get(url string) (resp *http.Response, err error) {
	return c.client.Get(url)
}
