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

	"github.com/aws/amazon-ecs-agent/ecs-init/config"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	orwPerm              = 0700
	regionalBucketFormat = "%s-%s"
)

// CacheStatus represents the status of the on-disk cache for agent
// tarballs in the cache directory. This status may be used to
// determine what the appropriate actions are based on the
// availability of agent images and communicates advice from the
// cache's state file.
type CacheStatus uint8

const (
	// StatusUncached indicates that there is not an already downloaded
	// and cached agent that is suitable for loading.
	StatusUncached CacheStatus = 0
	// StatusCached indicates that there is an agent downloaded and
	// cached. This should be taken to mean that the image is suitable
	// for load if agent isn't already loaded.
	StatusCached CacheStatus = 1
	// StatusReloadNeeded indicates that the cached image should take
	// precedence over the already loaded agent image. This may be
	// specified by the packaging to cause ecs-init to load a package
	// distributed cached image on package installation, upgrades, or
	// downgrades.
	StatusReloadNeeded CacheStatus = 2
)

// Downloader is responsible for cache operations relating to downloading the agent
type Downloader struct {
	s3Downloader s3DownloaderAPI
	fs           fileSystem
	metadata     instanceMetadata
	region       string
}

// NewDownloader returns a Downloader with default dependencies
func NewDownloader() (*Downloader, error) {
	downloader := &Downloader{
		fs: &standardFS{},
	}

	if config.RunningInExternal() {
		downloader.metadata = &blackholeInstanceMetadata{}
	} else {
		sessionInstance, err := session.NewSession()
		if err != nil {
			// metadata client is only used for retrieving the user's region.
			// If it cannot be initialized, the region field is populated with the default value to prevent future
			// calls to retrieve the region from metadata.
			log.Warnf("Got error when initializing session for metadata client: %v. Use default region: %s",
				err, config.DefaultRegionName)
			downloader.region = config.DefaultRegionName
		} else {
			downloader.metadata = ec2metadata.New(sessionInstance)
		}
	}

	s3Downloader := &s3Downloader{
		bucketDownloaders: make([]*s3BucketDownloader, 0),
		cacheDir:          config.CacheDirectory(),
		fs:                downloader.fs,
	}

	partitionBucketRegion := downloader.getPartitionBucketRegion()
	partitionBucket := config.AgentPartitionBucketName
	partitionBucketDownloader, err := newS3BucketDownloader(partitionBucketRegion, partitionBucket)
	if err != nil {
		log.Warnf("Failed to initialize partition bucket downloader: %v", err)
	} else {
		s3Downloader.addBucketDownloader(partitionBucketDownloader)
	}

	region := downloader.getRegion()
	regionalBucket := fmt.Sprintf(regionalBucketFormat, partitionBucket, region)
	regionalBucketDownloader, err := newS3BucketDownloader(region, regionalBucket)
	if err != nil {
		log.Warnf("Failed to initialize regional bucket downloader: %v", err)
	} else {
		s3Downloader.addBucketDownloader(regionalBucketDownloader)
	}

	if len(s3Downloader.bucketDownloaders) == 0 {
		log.Error("Failed to initialize s3 downloader for either partition bucket or regional bucket. Downloader initialization fails.")
		return nil, errors.New("failed to initialize downloader")
	}

	downloader.s3Downloader = s3Downloader
	return downloader, nil
}

// AgentCacheStatus inspects the on-disk cache and returns its
// status. See `CacheStatus` for possible cache statuses and
// scenarios.
func (d *Downloader) AgentCacheStatus() CacheStatus {
	stateFile := config.CacheState()
	// State file and tarball must be non-zero to report status on
	uncached := !(d.fileNotEmpty(stateFile) && d.fileNotEmpty(config.AgentTarball()))
	if uncached {
		return StatusUncached
	}

	file, err := d.fs.Open(stateFile)
	if err != nil {
		return StatusUncached
	}
	var status CacheStatus
	_, err = fmt.Fscanf(file, "%d", &status)
	if err != nil {
		return StatusUncached
	}
	return status
}

// IsAgentCached returns true if there is a cached copy of the Agent present
// and a cache state file is not empty (no validation is performed on the
// tarball or cache state file contents)
func (d *Downloader) IsAgentCached() bool {
	switch d.AgentCacheStatus() {
	case StatusUncached:
		return false
	}
	return true
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

	defaultRegion := config.DefaultRegionName
	if config.RunningInExternal() {
		region := os.Getenv(config.DefaultRegionEnvVar)
		if region == "" {
			log.Warnf("%s is not specified while running in external (non-EC2) environment. Using default region: %s",
				config.DefaultRegionEnvVar, defaultRegion)
		}
		d.region = defaultRegion
		return d.region
	}

	region, err := d.metadata.Region()
	if err != nil {
		log.Warnf("Could not retrieve the region from EC2 Instance Metadata. Error: %s", err.Error())
		region = defaultRegion
	}
	d.region = region

	return d.region
}

// getBucketRegion returns a region that contains the agent's bucket
func (d *Downloader) getPartitionBucketRegion() string {
	region := d.getRegion()

	destination, err := config.GetAgentPartitionBucketRegion(region)
	if err != nil {
		log.Warnf("Current region not supported, using the default region (%s) for downloader, err: %v", config.DefaultRegionName, err)
		return config.DefaultRegionName
	}

	return destination
}

// DownloadAgent downloads a copy of the Agent and performs an
// integrity check of the downloaded image
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
	log.Debugf("Expected MD5 %q", publishedMd5Sum)
	log.Debugf("Calculated MD5 %q", calculatedMd5SumString)
	if publishedMd5Sum != calculatedMd5SumString {
		agentTarballName, err := config.AgentRemoteTarballKey()
		if err != nil {
			return errors.New("downloaded agent does not match expected checksum")
		}
		return errors.Errorf("downloaded agent %q does not match expected checksum", agentTarballName)
	}

	log.Debugf("Attempting to rename %s to %s", tempFileName, config.AgentTarball())
	return d.fs.Rename(tempFileName, config.AgentTarball())
}

func (d *Downloader) getPublishedMd5Sum() (string, error) {
	objectKey, err := config.AgentRemoteTarballMD5Key()
	if err != nil {
		return "", errors.Wrap(err, "failed to determine md5 file for download")
	}
	tempMd5FileName, err := d.s3Downloader.downloadFile(objectKey)
	if err != nil {
		return "", errors.Wrap(err, "failed to download md5 file for published tarball")
	}

	tempMd5File, err := d.fs.Open(tempMd5FileName)
	if err != nil {
		return "", errors.Wrap(err, "failed to open temporary md5 file")
	}
	defer func() { // clean up temp file
		log.Debugf("Removing temp file %s", tempMd5FileName)
		d.fs.Remove(tempMd5FileName)
	}()

	body, err := d.fs.ReadAll(tempMd5File)
	if err != nil {
		return "", errors.Wrap(err, "failed to read from temporary md5 file")
	}

	return strings.TrimSpace(string(body)), nil
}

func (d *Downloader) getPublishedTarball() (string, error) {
	objectKey, err := config.AgentRemoteTarballKey()
	if err != nil {
		return "", errors.Wrap(err, "failed to determine download tarball")
	}
	tempAgentFileName, err := d.s3Downloader.downloadFile(objectKey)
	if err != nil {
		return "", errors.Wrap(err, "failed to download published tarball")
	}

	return tempAgentFileName, nil
}

// LoadCachedAgent returns an io.ReadCloser of the Agent from the cache
func (d *Downloader) LoadCachedAgent() (io.ReadCloser, error) {
	return d.fs.Open(config.AgentTarball())
}

// RecordCachedAgent writes the StatusCached state to disk to record a newly
// cached or loaded agent image; this prevents StatusReloadNeeded from
// being interpreted after the reload.
func (d *Downloader) RecordCachedAgent() error {
	data := []byte(fmt.Sprintf("%d", StatusCached))
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
