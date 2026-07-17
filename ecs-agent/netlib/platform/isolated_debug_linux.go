package platform

// isolatedDebug is the EC2 debug-mode variant of the isolated platform. All
// behavior, including the host resolv.conf DNS backfill, is inherited from
// isolatedLinux.
type isolatedDebug struct {
	isolatedLinux
}
