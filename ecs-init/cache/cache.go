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

// Package cache provides functionality for working with an on-disk cache of
// the ECS Agent image.
package cache

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/amazon-ecs-init/ecs-init/config"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	log "github.com/cihub/seelog"
)

const (
	orwPerm = 0700
)

// Downloader is responsible for cache operations relating to downloading the agent
type Downloader struct {
	s3downloader s3Downloader
	fs           fileSystem
	metadata     instanceMetadata
	region       string
}

// NewDownloader returns a Downloader with default dependencies
func NewDownloader() (*Downloader, error) {
	downloader := &Downloader{
		fs: &standardFS{},
	}

	// If metadata cannot be initialized the region string is populated with the default value to prevent future
	// calls to retrieve the region from metadata
	sessionInstance, err := session.NewSession()
	if err != nil {
		log.Debugf("Get error when initializing session instance: %v. Use default region: %s",
			err, config.DefaultRegionName)
		downloader.region = config.DefaultRegionName
	} else {
		// metadata is only used for retrieving the user's region
		downloader.metadata = ec2metadata.New(sessionInstance)
	}

	log.Debugf("Setting region for s3 client to: %s", downloader.getBucketRegion())
	s3Session, err := session.NewSession(&aws.Config{
		Credentials: credentials.AnonymousCredentials,
		Region:      aws.String(downloader.getBucketRegion()),
	})
	if err != nil {
		return nil, err
	}
	downloader.s3downloader = s3manager.NewDownloader(s3Session)

	return downloader, nil
}

// IsAgentCached returns true if there is a cached copy of the Agent present
// and a cache state file is not empty (no validation is performed on the
// tarball or cache state file contents)
func (d *Downloader) IsAgentCached() bool {
	return d.fileNotEmpty(config.CacheState()) && d.fileNotEmpty(config.AgentTarball())
}

func (d *Downloader) fileNotEmpty(filename string) bool {
	fileinfo, err := d.fs.Stat(filename)
	if err != nil {
		return false
	}
	return fileinfo.Size() > 0
}

// getRegion finds region from metadata and caches for the life of downloader
func (d *Downloader) getRegion() string {
	if d.region != "" {
		return d.region
	}

	region, err := d.metadata.Region()
	if err != nil {
		log.Warn("Could not retrieve the region from EC2 Instance Metadata. Error: %s", err.Error())
		region = config.DefaultRegionName
	}
	d.region = region

	return d.region
}

// getBucketRegion returns a region that contains the agent's bucket
func (d *Downloader) getBucketRegion() string {
	region := d.getRegion()

	destination, err := config.GetAgentBucketRegion(region)
	if err != nil {
		log.Warnf("Current region not supported, using the default region (%s) for downloader, err: %v", config.DefaultRegionName, err)
		return config.DefaultRegionName
	}

	return destination
}

// DownloadAgent downloads a fresh copy of the Agent and performs an
// integrity check on the downloaded image
func (d *Downloader) DownloadAgent() error {
	err := d.fs.MkdirAll(config.CacheDirectory(), os.ModeDir|orwPerm)
	if err != nil {
		return err
	}

	publishedMd5Sum, err := d.getPublishedMd5Sum()
	if err != nil {
		return err
	}

	tempFileName, err := d.getPublishedTarball()
	if err != nil {
		return err
	}

	defer func() { // clean up temp file
		if _, err := d.fs.Stat(tempFileName); err == nil { // if temp file exists, remove it
			log.Debugf("Removing temp file %s", tempFileName)
			d.fs.Remove(tempFileName)
		}
	}()

	publishedTarballReader, err := d.fs.Open(tempFileName)
	if err != nil {
		return err
	}
	defer publishedTarballReader.Close()

	md5hash := md5.New()
	_, err = d.fs.Copy(md5hash, publishedTarballReader)
	if err != nil {
		return err
	}

	calculatedMd5Sum := md5hash.Sum(nil)
	calculatedMd5SumString := fmt.Sprintf("%x", calculatedMd5Sum)
	log.Debugf("Expected %s", publishedMd5Sum)
	log.Debugf("Calculated %s", calculatedMd5SumString)
	agentRemoteTarball := config.AgentRemoteTarballKey()
	if publishedMd5Sum != calculatedMd5SumString {
		err = fmt.Errorf("mismatched md5sum while downloading %s", agentRemoteTarball)
		return err
	}

	log.Debugf("Attempting to rename %s to %s", tempFileName, config.AgentTarball())
	return d.fs.Rename(tempFileName, config.AgentTarball())
}

func (d *Downloader) getPublishedMd5Sum() (string, error) {
	tempFile, err := d.fs.TempFile(config.CacheDirectory(), "ecs-agent.tar.md5")
	if err != nil {
		return "", err
	}
	log.Debugf("Created temporary file for md5sum: %s", tempFile.Name())
	defer func() {
		log.Debugf("Removing temp file %s", tempFile.Name())
		d.fs.Remove(tempFile.Name())
	}()
	defer tempFile.Close()

	_, err = d.s3downloader.Download(tempFile, &s3.GetObjectInput{
		Bucket: aws.String(config.AgentRemoteBucketName),
		Key:    aws.String(config.AgentRemoteTarballMD5Key()),
	})
	if err != nil {
		return "", err
	}

	body, err := d.fs.ReadAll(tempFile)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(body)), nil
}

func (d *Downloader) getPublishedTarball() (string, error) {
	tempFile, err := d.fs.TempFile(config.CacheDirectory(), "ecs-agent.tar")
	if err != nil {
		return "", err
	}
	log.Debugf("Created temporary file for agent tarball: %s", tempFile.Name())

	_, err = d.s3downloader.Download(tempFile, &s3.GetObjectInput{
		Bucket: aws.String(config.AgentRemoteBucketName),
		Key:    aws.String(config.AgentRemoteTarballKey()),
	})
	if err != nil {
		tempFile.Close()
		log.Debugf("Removing temp file %s", tempFile.Name())
		d.fs.Remove(tempFile.Name())
		return "", err
	}

	tempFile.Close()
	return tempFile.Name(), nil
}

// LoadCachedAgent returns an io.ReadCloser of the Agent from the cache
func (d *Downloader) LoadCachedAgent() (io.ReadCloser, error) {
	return d.fs.Open(config.AgentTarball())
}

func (d *Downloader) RecordCachedAgent() error {
	data := []byte("1")
	return d.fs.WriteFile(config.CacheState(), data, orwPerm)
}

// LoadDesiredAgent returns an io.ReadCloser of the Agent indicated by the desiredImageLocatorFile
// (/var/cache/ecs/desired-image). The desiredImageLocatorFile must contain as the beginning of the file the name of
// the file containing the desired image (interpreted as a basename) and ending in a newline.  Only the first line is
// read, with the rest of the file reserved for future use.
func (d *Downloader) LoadDesiredAgent() (io.ReadCloser, error) {
	desiredImageFile, err := d.getDesiredImageFile()
	if err != nil {
		return nil, err
	}
	return d.fs.Open(desiredImageFile)
}

func (d *Downloader) getDesiredImageFile() (string, error) {
	file, err := d.fs.Open(config.DesiredImageLocatorFile())
	if err != nil {
		return "", err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	desiredImageString, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	desiredImageFile := strings.TrimSpace(config.CacheDirectory() + "/" + d.fs.Base(desiredImageString))
	return desiredImageFile, nil
}
