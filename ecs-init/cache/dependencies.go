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

//go:generate mockgen.sh cache $GOFILE

import (
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// s3API captures the only method used from the s3 package
type s3API interface {
	Download(w io.WriterAt, input *s3.GetObjectInput, options ...func(*s3manager.Downloader)) (n int64, err error)
}

// s3BucketDownloader wraps a bucket together with a downloader that can download from it
type s3BucketDownloader struct {
	bucket string
	region string
	client s3API
}

func newS3BucketDownloader(region, bucketName string) (*s3BucketDownloader, error) {
	session, err := session.NewSession(&aws.Config{
		Credentials: credentials.AnonymousCredentials,
		Region:      aws.String(region),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to initialize downloader in region %s", region)
	}

	s3BucketDownloader := &s3BucketDownloader{
		client: s3manager.NewDownloader(session),
		bucket: bucketName,
		region: region,
	}

	return s3BucketDownloader, nil
}

func (bd *s3BucketDownloader) download(fileName, cacheDir string, fs fileSystem) (string, error) {
	file, err := fs.TempFile(cacheDir, fileName)
	if err != nil {
		return "", errors.Wrap(err, "could not create local file during download")
	}

	defer func() { // make sure we also handle possible error from f.Close()
		cerr := file.Close()
		if err == nil { // if no error return from download, captures error from f.Close()
			err = cerr
		}
	}()

	_, err = bd.client.Download(file, &s3.GetObjectInput{
		Bucket: aws.String(bd.bucket),
		Key:    aws.String(fileName),
	})

	return file.Name(), err
}

type s3DownloaderAPI interface {
	addBucketDownloader(bucketDownloader *s3BucketDownloader)
	downloadFile(fileName string) (string, error)
}

type s3Downloader struct {
	bucketDownloaders []*s3BucketDownloader
	fs                fileSystem
	cacheDir          string
}

func (d *s3Downloader) addBucketDownloader(bucketDownloader *s3BucketDownloader) {
	d.bucketDownloaders = append(d.bucketDownloaders, bucketDownloader)
}

func (d *s3Downloader) downloadFile(fileName string) (string, error) {
	for _, bucketDownloader := range d.bucketDownloaders {
		fileName, err := bucketDownloader.download(fileName, d.cacheDir, d.fs)
		if err == nil {
			log.Debugf("Download file %s from bucket %s in region %s succeeded.",
				fileName, bucketDownloader.bucket, bucketDownloader.region)
			return fileName, nil
		} else {
			log.Errorf("Download file %s from bucket %s in region %s failed with error: %v",
				fileName, bucketDownloader.bucket, bucketDownloader.region, err)
		}
	}

	log.Debugf("Failed to download file %s from s3", fileName)
	return "", errors.New("failed to download file from s3")
}

// fileSystem captures related functions from io and os packages
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

type blackholeInstanceMetadata struct {
}

func (b *blackholeInstanceMetadata) Region() (string, error) {
	return "", errors.New("blackholed")
}

// standardFS delegates to the package-level functions
type standardFS struct{}

func (s *standardFS) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (s *standardFS) TempFile(dir, prefix string) (*os.File, error) {
	return os.CreateTemp(dir, prefix)
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
	return io.ReadAll(r)
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
	return os.WriteFile(filename, data, perm)
}
