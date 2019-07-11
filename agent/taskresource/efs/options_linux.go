package efs

import (
	"errors"
	"strings"
)

var supportedNfsOpts = map[string]string{
	"resvport": "noresvport",
	"soft":     "hard",
	"ac":       "noac",
	"fg":       "bg",
	"intr":     "nointr",
	"cto":      "nocto",
	"rsize":    "",
	"wsize":    "",
	"timeo":    "",
	"retrans":  "",
}

// checkOptionSet validates customer input against the supported NFS options
// TODO: move to service layer exclusively?
func checkOptionSet(optionSet string) error {
	options := strings.Split(optionSet, ",")
	for _, option := range options {
		split := strings.Split(option, "=")
		found := false
		for k, v := range supportedNfsOpts {
			if k == split[0] || v == split[0] {
				found = true
				break
			}
		}
		if !found {
			return errors.New("unsupported nfs option: " + split[0])
		}
	}
	return nil
}

// mergeOptions collapses sets of options into one, preferring the option to
// the right. Order is not preserved. Flag values (ie, for the 'ro' flag) are not extracted.
func mergeOptions(sets ...string) string {
	valueMap := make(map[string]string)
	optMap := make(map[string]bool)
	for _, options := range sets {
		for _, option := range strings.Split(options, ",") {
			split := strings.Split(option, "=")
			key := split[0]
			if key == "" {
				continue
			}
			normalizedKey, reverse := parseMountOptions(key)

			optMap[normalizedKey] = reverse

			if len(split) > 1 {
				valueMap[normalizedKey] = split[1]
			}
		}
	}

	b := strings.Builder{}
	// we don't want to add a comma to the first option -- but every subsequent option needs to be prepended with a comma
	addComma := false

	// 'shouldReverse' means we should use the right-hand-side (value) of the supportedNfsOpts map
	for normalizedKey, shouldReverse := range optMap {
		if addComma {
			b.WriteString(",")
		}
		if shouldReverse {
			b.WriteString(supportedNfsOpts[normalizedKey])
		} else {
			b.WriteString(normalizedKey)
		}
		if value, ok := valueMap[normalizedKey]; ok {
			b.WriteString("=")
			b.WriteString(value)
		}
		addComma = true
	}
	return b.String()
}

// parseMountOptions takes a mount option and returns true if its discovered as an NFS option. Some options also have
// inverse options (eg, "cto" vs "nocto") so we normalize that and return a boolean value if it should be reversed.
func parseMountOptions(mountOpt string) (normalizedKey string, reverse bool) {
	if mountOpt == "" {
		return
	}

	normalizedKey = mountOpt
	for k, v := range supportedNfsOpts {
		if mountOpt == k {
			return
		}
		if mountOpt == v {
			normalizedKey = k
			reverse = true
			return
		}
	}
	return
}
