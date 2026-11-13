//go:build linux
// +build linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package driver

import (
	"fmt"
	"strings"

	"k8s.io/klog/v2"
)

const (
	// filesystemFeaturesPrefix is the dumpe2fs -h line that lists superblock
	// features. It is distinct from the "Journal features:" line.
	filesystemFeaturesPrefix = "Filesystem features:"
	// featureNeedsRecovery is set while the ext3/ext4 journal has entries that
	// have not been replayed yet.
	featureNeedsRecovery = "needs_recovery"
)

// FormatAndMountSensitiveWithFormatOptions shadows the method promoted from the
// embedded mountutils.SafeFormatAndMount. Before delegating, it makes sure that
// an existing ext2/ext3/ext4 filesystem has no pending journal recovery, so that
// journal replay never happens in the host kernel during mount. Legitimate
// volumes have their journal replayed by fsck in userspace, which clears
// needs_recovery; anything still requiring recovery is refused.
func (m *NodeMounter) FormatAndMountSensitiveWithFormatOptions(source string, target string, fstype string,
	options []string, sensitiveOptions []string, formatOptions []string) error {
	if err := m.ensureNoPendingJournalRecovery(source, isReadOnlyMount(options, sensitiveOptions)); err != nil {
		return err
	}
	return m.SafeFormatAndMount.FormatAndMountSensitiveWithFormatOptions(source, target, fstype,
		options, sensitiveOptions, formatOptions)
}

// ensureNoPendingJournalRecovery returns an error if source holds an
// ext2/ext3/ext4 filesystem that still needs journal recovery. It keys off the
// on-disk filesystem type reported by blkid rather than the requested fstype,
// and fails closed when the superblock cannot be inspected.
func (m *NodeMounter) ensureNoPendingJournalRecovery(source string, readOnly bool) error {
	existingFormat, err := m.GetDiskFormat(source)
	if err != nil {
		return fmt.Errorf("failed to get disk format of disk %s: %w", source, err)
	}
	if !isExtFilesystem(existingFormat) {
		return nil
	}

	if !readOnly {
		// Let fsck replay the journal in userspace. The result is intentionally
		// ignored here; the superblock check below is authoritative, and the
		// delegated upstream mount still runs fsck and enforces its exit codes.
		if out, fsckErr := m.Exec.Command("fsck", "-a", source).CombinedOutput(); fsckErr != nil {
			klog.InfoS("fsck before journal recovery check returned an error", "source", source,
				"error", fsckErr, "output", string(out))
		}
	}

	out, err := m.Exec.Command("dumpe2fs", "-h", source).CombinedOutput()
	if err != nil {
		return fmt.Errorf("refusing to mount %s: failed to read %s superblock: %w", source, existingFormat, err)
	}
	features, ok := parseFilesystemFeatures(string(out))
	if !ok {
		err = fmt.Errorf("refusing to mount %s: could not find filesystem features of %s superblock",
			source, existingFormat)
		klog.ErrorS(err, "dumpe2fs output has no filesystem features line", "source", source,
			"fstype", existingFormat, "output", string(out))
		return err
	}
	if _, pending := features[featureNeedsRecovery]; pending {
		return fmt.Errorf("refusing to mount %s: %s filesystem still needs journal recovery", source, existingFormat)
	}
	return nil
}

// parseFilesystemFeatures returns the set of features on the "Filesystem
// features:" line of dumpe2fs -h output, and whether that line was found.
func parseFilesystemFeatures(dumpe2fsOutput string) (map[string]struct{}, bool) {
	for _, line := range strings.Split(dumpe2fsOutput, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, filesystemFeaturesPrefix) {
			continue
		}
		features := map[string]struct{}{}
		for _, f := range strings.Fields(strings.TrimPrefix(line, filesystemFeaturesPrefix)) {
			features[f] = struct{}{}
		}
		return features, true
	}
	return nil, false
}

func isExtFilesystem(fstype string) bool {
	switch fstype {
	case FSTypeExt2, FSTypeExt3, FSTypeExt4:
		return true
	}
	return false
}

// isReadOnlyMount mirrors how mountutils.SafeFormatAndMount detects read-only
// mounts.
func isReadOnlyMount(options []string, sensitiveOptions []string) bool {
	for _, opts := range [][]string{options, sensitiveOptions} {
		for _, o := range opts {
			if o == "ro" {
				return true
			}
		}
	}
	return false
}
