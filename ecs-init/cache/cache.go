// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

// Package cache provides functionality for working with an on-disk cache of
// the ECS Agent image.
package cache

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	log "github.com/cihub/seelog"
)

const (
	cacheDirectory        = "/tmp/var/cache/ecs"
	agentTarball          = cacheDirectory + "/ecs-agent.tar"
	agentRemoteTarball    = "https://s3.amazonaws.com/amazon-ecs-agent/ecs-agent-latest.tar"
	agentRemoteTarballMD5 = agentRemoteTarball + ".md5"
)

// Downloader is resposible for cache operations relating to downloading the agent
type Downloader struct {
	getter httpGetter
	fs     fileSystem
}

// NewDownloader returns a Downloader with default dependencies
func NewDownloader() *Downloader {
	return &Downloader{
		getter: customGetter,
		fs:     &standardFS{},
	}
}

// IsAgentCached returns true if there is a cached copy of the Agent present
// (no validation is performed)
func (d *Downloader) IsAgentCached() bool {
	file, err := d.fs.Open(agentTarball)
	if err != nil {
		return false
	}
	file.Close()
	return true
}

// IsAgentLatest checks whether the cached copy of the Agent has the same MD5
// sum as the published MD5 sum
func (d *Downloader) IsAgentLatest() bool {
	file, err := d.fs.Open(agentTarball)
	if err != nil {
		return false
	}
	defer file.Close()

	md5hash := md5.New()
	_, err = d.fs.Copy(md5hash, file)
	if err != nil {
		log.Error("Could not calculate md5sum", err)
		return false
	}

	publishedMd5Sum, err := d.getPublishedMd5Sum()
	if err != nil {
		return false
	}

	calculatedMd5Sum := md5hash.Sum(nil)
	calculatedMd5SumString := fmt.Sprintf("%x", calculatedMd5Sum)
	log.Infof("Expected %s", publishedMd5Sum)
	log.Infof("Calculated %s", calculatedMd5SumString)
	if publishedMd5Sum != calculatedMd5SumString {
		log.Info("Cached Amazon EC2 Container Service Agent does not match latest at %s", agentRemoteTarball)
		return false
	}
	return true
}

// LoadCachedAgent returns an io.ReadCloser of the Agent from the cache
func (d *Downloader) LoadCachedAgent() (io.ReadCloser, error) {
	return d.fs.Open(agentTarball)
}

// DownloadAgent downloads a fresh copy of the Agent and performs an
// integrity check on the downloaded image
func (d *Downloader) DownloadAgent() error {
	err := d.fs.MkdirAll(cacheDirectory, os.ModeDir|0700)
	if err != nil {
		return err
	}

	publishedMd5Sum, err := d.getPublishedMd5Sum()
	if err != nil {
		return err
	}

	publishedTarballReader, err := d.getPublishedTarball()
	if err != nil {
		return err
	}
	defer publishedTarballReader.Close()

	md5hash := md5.New()
	tempFile, err := d.fs.TempFile("", "ecs-agent.tar")
	if err != nil {
		return err
	}
	defer tempFile.Close()

	log.Debugf("Temp file %s", tempFile.Name())
	defer func() {
		if err != nil {
			log.Debugf("Removing temp file %s", tempFile.Name())
			d.fs.Remove(tempFile.Name())
		}
	}()

	teeReader := d.fs.TeeReader(publishedTarballReader, md5hash)
	_, err = d.fs.Copy(tempFile, teeReader)
	if err != nil {
		return err
	}

	calculatedMd5Sum := md5hash.Sum(nil)
	calculatedMd5SumString := fmt.Sprintf("%x", calculatedMd5Sum)
	log.Infof("Expected %s", publishedMd5Sum)
	log.Infof("Calculated %s", calculatedMd5SumString)
	if publishedMd5Sum != calculatedMd5SumString {
		err = fmt.Errorf("mismatched md5sum while downloading %s", agentRemoteTarball)
		return err
	}

	log.Debugf("Attempting to rename %s to %s", tempFile.Name(), agentTarball)
	return d.fs.Rename(tempFile.Name(), agentTarball)
}

func (d *Downloader) getPublishedMd5Sum() (string, error) {
	log.Infof("Downloading published md5sum from %s", agentRemoteTarballMD5)
	resp, err := d.getter.Get(agentRemoteTarballMD5)
	if err != nil {
		return "", err
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()
	body, err := d.fs.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

func (d *Downloader) getPublishedTarball() (io.ReadCloser, error) {
	log.Infof("Downloading Amazon EC2 Container Service Agent from %s", agentRemoteTarball)
	resp, err := d.getter.Get(agentRemoteTarball)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response code %d", resp.StatusCode)
	}
	return resp.Body, nil
}
