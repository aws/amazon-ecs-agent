package ecscni

const (
	NETNS_PATH_DEFAULT = "/var/run/netns"
	NETNS_PROC_FORMAT  = "/proc/%d/task/%d/ns/net"

	nsFileMode = 0444
)

// Config is a general interface represents all kinds of plugin configs
type Config interface {
	String() string
}

// CNIPluginVersion is used to convert the JSON output of the
// '--version' command into a string
type CNIPluginVersion struct {
	Version string `json:"version"`
	Dirty   bool   `json:"dirty"`
	Hash    string `json:"gitShortHash"`
}

// String returns the version information as formatted string
func (v *CNIPluginVersion) String() string {
	ver := ""
	if v.Dirty {
		ver = "@"
	}

	return ver + v.Hash + "-" + v.Version
}
