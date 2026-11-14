//go:build linux && unit
// +build linux,unit

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
	"errors"
	"reflect"
	"strings"
	"testing"

	"k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
	fakeexec "k8s.io/utils/exec/testing"
)

const (
	testSource = "/dev/nvme1n1"
	testTarget = "/var/lib/ebs/staging"

	dumpe2fsClean = `Filesystem volume name:   <none>
Filesystem magic number:  0xEF53
Filesystem features:      has_journal ext_attr resize_inode dir_index filetype extent 64bit flex_bg sparse_super large_file huge_file dir_nlink extra_isize metadata_csum
Journal features:         journal_incompat_revoke journal_64bit journal_checksum_v3
`
	dumpe2fsNeedsRecovery = `Filesystem volume name:   <none>
Filesystem magic number:  0xEF53
Filesystem features:      has_journal ext_attr resize_inode dir_index filetype needs_recovery extent 64bit flex_bg sparse_super large_file huge_file dir_nlink extra_isize metadata_csum
Journal features:         journal_incompat_revoke journal_64bit journal_checksum_v3
`
	dumpe2fsNoFeatures = `Filesystem volume name:   <none>
Filesystem magic number:  0xEF53
Journal features:         needs_recovery
`
)

// fakeStep is one expected command invocation and its scripted result.
type fakeStep struct {
	cmd    string
	output string
	err    error
}

func blkidStep(fstype string) fakeStep {
	if fstype == "" {
		return fakeStep{cmd: "blkid", err: &fakeexec.FakeExitError{Status: 2}}
	}
	return fakeStep{cmd: "blkid", output: "DEVNAME=" + testSource + "\nTYPE=" + fstype + "\n"}
}

// newGuardTestMounter returns a NodeMounter backed by a fake mounter and a fake
// exec that serves steps in order, plus a pointer to the recorded commands.
func newGuardTestMounter(t *testing.T, steps []fakeStep) (*NodeMounter, *mount.FakeMounter, *[]string) {
	t.Helper()
	var called []string
	fexec := &fakeexec.FakeExec{ExactOrder: true}
	for _, s := range steps {
		s := s
		fexec.CommandScript = append(fexec.CommandScript, func(cmd string, args ...string) utilexec.Cmd {
			called = append(called, cmd)
			if cmd != s.cmd {
				t.Errorf("unexpected command %q %v, expected %q", cmd, args, s.cmd)
			}
			fcmd := &fakeexec.FakeCmd{
				CombinedOutputScript: []fakeexec.FakeAction{
					func() ([]byte, []byte, error) { return []byte(s.output), nil, s.err },
				},
			}
			return fakeexec.InitFakeCmd(fcmd, cmd, args...)
		})
	}
	fm := mount.NewFakeMounter(nil)
	return &NodeMounter{&mount.SafeFormatAndMount{Interface: fm, Exec: fexec}}, fm, &called
}

func TestFormatAndMountJournalRecoveryGuard(t *testing.T) {
	testCases := []struct {
		name         string
		fstype       string
		options      []string
		steps        []fakeStep
		expectErr    string
		expectMount  bool
		expectedCmds []string
	}{
		{
			name:   "ext4 rw clean after fsck mounts",
			fstype: FSTypeExt4,
			steps: []fakeStep{
				blkidStep("ext4"), {cmd: "fsck"}, {cmd: "dumpe2fs", output: dumpe2fsClean},
				blkidStep("ext4"), {cmd: "fsck"},
			},
			expectMount:  true,
			expectedCmds: []string{"blkid", "fsck", "dumpe2fs", "blkid", "fsck"},
		},
		{
			name:   "ext4 rw still needs recovery after fsck is refused",
			fstype: FSTypeExt4,
			steps: []fakeStep{
				blkidStep("ext4"), {cmd: "fsck", err: &fakeexec.FakeExitError{Status: 8}},
				{cmd: "dumpe2fs", output: dumpe2fsNeedsRecovery},
			},
			expectErr:    "still needs journal recovery",
			expectedCmds: []string{"blkid", "fsck", "dumpe2fs"},
		},
		{
			name:    "ext4 ro needing recovery is refused without running fsck",
			fstype:  FSTypeExt4,
			options: []string{"ro"},
			steps: []fakeStep{
				blkidStep("ext4"), {cmd: "dumpe2fs", output: dumpe2fsNeedsRecovery},
			},
			expectErr:    "still needs journal recovery",
			expectedCmds: []string{"blkid", "dumpe2fs"},
		},
		{
			name:    "ext4 ro clean mounts without running fsck",
			fstype:  FSTypeExt4,
			options: []string{"ro"},
			steps: []fakeStep{
				blkidStep("ext4"), {cmd: "dumpe2fs", output: dumpe2fsClean},
				blkidStep("ext4"),
			},
			expectMount:  true,
			expectedCmds: []string{"blkid", "dumpe2fs", "blkid"},
		},
		{
			name:   "ext3 needing recovery is refused",
			fstype: FSTypeExt3,
			steps: []fakeStep{
				blkidStep("ext3"), {cmd: "fsck"}, {cmd: "dumpe2fs", output: dumpe2fsNeedsRecovery},
			},
			expectErr:    "still needs journal recovery",
			expectedCmds: []string{"blkid", "fsck", "dumpe2fs"},
		},
		{
			name:   "ext2 clean mounts",
			fstype: FSTypeExt2,
			steps: []fakeStep{
				blkidStep("ext2"), {cmd: "fsck"}, {cmd: "dumpe2fs", output: dumpe2fsClean},
				blkidStep("ext2"), {cmd: "fsck"},
			},
			expectMount:  true,
			expectedCmds: []string{"blkid", "fsck", "dumpe2fs", "blkid", "fsck"},
		},
		{
			name:   "requested xfs but on-disk ext4 needing recovery is refused",
			fstype: FSTypeXfs,
			steps: []fakeStep{
				blkidStep("ext4"), {cmd: "fsck"}, {cmd: "dumpe2fs", output: dumpe2fsNeedsRecovery},
			},
			expectErr:    "still needs journal recovery",
			expectedCmds: []string{"blkid", "fsck", "dumpe2fs"},
		},
		{
			name:   "dumpe2fs failure fails closed",
			fstype: FSTypeExt4,
			steps: []fakeStep{
				blkidStep("ext4"), {cmd: "fsck"},
				{cmd: "dumpe2fs", output: "bad magic", err: &fakeexec.FakeExitError{Status: 1}},
			},
			expectErr:    "failed to read ext4 superblock",
			expectedCmds: []string{"blkid", "fsck", "dumpe2fs"},
		},
		{
			name:   "missing filesystem features line fails closed",
			fstype: FSTypeExt4,
			steps: []fakeStep{
				blkidStep("ext4"), {cmd: "fsck"}, {cmd: "dumpe2fs", output: dumpe2fsNoFeatures},
			},
			expectErr:    "could not find filesystem features",
			expectedCmds: []string{"blkid", "fsck", "dumpe2fs"},
		},
		{
			name:         "blkid failure is returned",
			fstype:       FSTypeExt4,
			steps:        []fakeStep{{cmd: "blkid", err: errors.New("blkid exploded")}},
			expectErr:    "failed to get disk format",
			expectedCmds: []string{"blkid"},
		},
		{
			name:   "xfs skips the guard",
			fstype: FSTypeXfs,
			steps: []fakeStep{
				blkidStep("xfs"),
				blkidStep("xfs"), {cmd: "fsck"},
			},
			expectMount:  true,
			expectedCmds: []string{"blkid", "blkid", "fsck"},
		},
		{
			name:   "unformatted disk skips the guard and is formatted",
			fstype: FSTypeExt4,
			steps: []fakeStep{
				blkidStep(""),
				blkidStep(""), {cmd: "mkfs.ext4"},
			},
			expectMount:  true,
			expectedCmds: []string{"blkid", "blkid", "mkfs.ext4"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m, fm, called := newGuardTestMounter(t, tc.steps)

			err := m.FormatAndMountSensitiveWithFormatOptions(testSource, testTarget, tc.fstype, tc.options, nil, nil)

			if tc.expectErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.expectErr) {
					t.Fatalf("expected error containing %q, got %v", tc.expectErr, err)
				}
			} else if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			mounted := len(fm.MountPoints) > 0
			if mounted != tc.expectMount {
				t.Fatalf("expected mounted=%v, got mount points %v", tc.expectMount, fm.MountPoints)
			}
			if !reflect.DeepEqual(*called, tc.expectedCmds) {
				t.Fatalf("expected commands %v, got %v", tc.expectedCmds, *called)
			}
		})
	}
}

func TestParseFilesystemFeatures(t *testing.T) {
	features, ok := parseFilesystemFeatures(dumpe2fsNeedsRecovery)
	if !ok {
		t.Fatal("expected filesystem features line to be found")
	}
	for _, f := range []string{"has_journal", "needs_recovery", "metadata_csum"} {
		if _, found := features[f]; !found {
			t.Errorf("expected feature %q in %v", f, features)
		}
	}
	if _, found := features["journal_64bit"]; found {
		t.Errorf("journal features must not be parsed as filesystem features: %v", features)
	}

	if _, ok := parseFilesystemFeatures(dumpe2fsNoFeatures); ok {
		t.Error("expected no filesystem features line to be found")
	}
}

func TestIsReadOnlyMount(t *testing.T) {
	if isReadOnlyMount([]string{"rw", "noatime"}, nil) {
		t.Error("expected rw mount")
	}
	if !isReadOnlyMount([]string{"noatime", "ro"}, nil) {
		t.Error("expected ro from options")
	}
	if !isReadOnlyMount(nil, []string{"ro"}) {
		t.Error("expected ro from sensitive options")
	}
}
