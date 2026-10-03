//go:build linux && sudo
// +build linux,sudo

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

package platform

import (
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise the introspection firewall against the REAL iptables /
// ip6tables binaries (no stubbing), asserting the actual netfilter state.
//
// The test mutates real kernel netfilter rules, so to avoid touching the host's
// firewall it first moves into a FRESH, EMPTY network namespace (see
// enterFreshNetNS). That gives clean, deterministic isolation: the INPUT chain
// starts empty (no state from a prior run to pre-clean, no leak to tear down),
// and the kernel reclaims the whole namespace — and every rule in it — when the
// locked OS thread exits. Nothing here can affect the host or a concurrent test.
//
// These tests require root: creating a namespace needs CAP_SYS_ADMIN and
// modifying netfilter needs CAP_NET_ADMIN. Because the namespace provides the
// isolation, no container is needed — build the test binary as your normal user
// (so it uses the module/build cache) and run it under sudo:
//
//	go test -tags 'linux sudo' -c -o /tmp/introspect.test ./netlib/platform/
//	sudo /tmp/introspect.test -test.run Introspection -test.v
//
// The rules reference the bridge interface (BridgeInterfaceName) by name;
// iptables accepts -i on a non-existent interface, so the test does not need the
// bridge to exist — it asserts on rule presence and ordering in the INPUT chain.

// enterFreshNetNS moves the current goroutine into a new, empty network
// namespace and keeps it there for the rest of the test. It locks the goroutine
// to its OS thread and deliberately never unlocks: subprocesses (the exec'd
// iptables/ip6tables) inherit the thread's namespace, so pinning the thread is
// what guarantees those commands act on the fresh namespace rather than the
// host. When the test goroutine exits still locked, the Go runtime terminates
// the thread, which drops the last reference to the namespace and the kernel
// tears it (and all its netfilter rules) down — so no explicit cleanup is
// needed.
func enterFreshNetNS(t *testing.T) {
	t.Helper()
	runtime.LockOSThread() // intentionally never unlocked; see doc comment.
	if err := syscall.Unshare(syscall.CLONE_NEWNET); err != nil {
		t.Fatalf("unshare(CLONE_NEWNET) failed (needs CAP_SYS_ADMIN / root): %v", err)
	}
}

// iptablesBin returns the iptables binary for the given address family.
func iptablesBin(useIPv6 bool) string {
	if useIPv6 {
		return ipv6Tables
	}
	return iptablesExecutable
}

// daemonAddr returns the daemon namespace's fixed daemon-bridge source address
// for the given family — the source the ACCEPT rule matches on.
func daemonAddr(useIPv6 bool) string {
	if useIPv6 {
		return DaemonBridgeIPv6
	}
	return DaemonBridgeIP
}

// listINPUT returns the INPUT chain rule specs for the given family via
// `iptables -S INPUT` (or ip6tables), one rule per element.
func listINPUT(t *testing.T, useIPv6 bool) []string {
	t.Helper()
	bin := iptablesBin(useIPv6)
	// bin is a fixed iptables/ip6tables constant and the remaining args are
	// literals; exec.Command runs the binary directly (no shell). Test-only.
	// nosemgrep: command-injection-exec-variable
	out, err := exec.Command(bin, "-w", "-S", "INPUT").CombinedOutput()
	require.NoError(t, err, "%s -S INPUT failed: %s", bin, string(out))
	var rules []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) != "" {
			rules = append(rules, line)
		}
	}
	return rules
}

// isIntrospectionRule reports whether an `iptables -S INPUT` rule spec is one of
// our introspection rules with the given verdict ("ACCEPT" or "DROP"): it must
// match the daemon-bridge interface and the introspection port. Matching by
// these tokens is robust to iptables' canonical formatting/reordering.
func isIntrospectionRule(rule, verdict string) bool {
	return strings.Contains(rule, BridgeInterfaceName) &&
		strings.Contains(rule, introspectionServerPort) &&
		strings.Contains(rule, verdict)
}

// dropRuleIndex returns the INPUT-chain position of the baseline DROP rule, or
// -1 if it is absent.
func dropRuleIndex(rules []string) int {
	for i, r := range rules {
		if isIntrospectionRule(r, "DROP") {
			return i
		}
	}
	return -1
}

// acceptRuleIndex returns the INPUT-chain position of the daemon ACCEPT rule for
// the given family (matched by the daemon source address), or -1 if absent.
func acceptRuleIndex(rules []string, useIPv6 bool) int {
	for i, r := range rules {
		if isIntrospectionRule(r, "ACCEPT") && strings.Contains(r, daemonAddr(useIPv6)) {
			return i
		}
	}
	return -1
}

// countDropRules / countAcceptRules count matching rules, used to assert that a
// repeated setup/allow does not create duplicates.
func countDropRules(rules []string) int {
	n := 0
	for _, r := range rules {
		if isIntrospectionRule(r, "DROP") {
			n++
		}
	}
	return n
}

func countAcceptRules(rules []string, useIPv6 bool) int {
	n := 0
	for _, r := range rules {
		if isIntrospectionRule(r, "ACCEPT") && strings.Contains(r, daemonAddr(useIPv6)) {
			n++
		}
	}
	return n
}

func TestIntrospectionFirewall_Integration(t *testing.T) {
	// Isolate in a fresh, empty network namespace: the INPUT chain starts empty
	// and the kernel reclaims the namespace (and every rule below) when the test
	// thread exits, so there is no pre-clean or teardown to get right.
	enterFreshNetNS(t)

	// 1. Baseline DROP installed for both families.
	require.NoError(t, SetupIntrospectionFirewall(true))
	for _, useIPv6 := range []bool{false, true} {
		rules := listINPUT(t, useIPv6)
		assert.GreaterOrEqual(t, dropRuleIndex(rules), 0,
			"baseline DROP should be present (ipv6=%v): %v", useIPv6, rules)
	}

	// 2. Idempotent: re-running setup does not add a duplicate DROP.
	require.NoError(t, SetupIntrospectionFirewall(true))
	for _, useIPv6 := range []bool{false, true} {
		rules := listINPUT(t, useIPv6)
		assert.Equal(t, 1, countDropRules(rules),
			"exactly one DROP after re-setup (ipv6=%v): %v", useIPv6, rules)
	}

	// 3. allowDaemonIntrospection inserts the ACCEPT AHEAD OF the DROP.
	require.NoError(t, allowDaemonIntrospection(true))
	for _, useIPv6 := range []bool{false, true} {
		rules := listINPUT(t, useIPv6)
		acceptIdx := acceptRuleIndex(rules, useIPv6)
		dropIdx := dropRuleIndex(rules)
		require.GreaterOrEqual(t, acceptIdx, 0, "ACCEPT present (ipv6=%v): %v", useIPv6, rules)
		require.GreaterOrEqual(t, dropIdx, 0, "DROP present (ipv6=%v): %v", useIPv6, rules)
		assert.Less(t, acceptIdx, dropIdx,
			"ACCEPT must be evaluated before DROP (ipv6=%v): %v", useIPv6, rules)
	}

	// 4. Idempotent: re-running Allow does not add a duplicate ACCEPT.
	require.NoError(t, allowDaemonIntrospection(true))
	for _, useIPv6 := range []bool{false, true} {
		rules := listINPUT(t, useIPv6)
		assert.Equal(t, 1, countAcceptRules(rules, useIPv6),
			"exactly one ACCEPT after re-allow (ipv6=%v): %v", useIPv6, rules)
	}

	// 5. disallowDaemonIntrospection removes ONLY the ACCEPT, leaving the DROP.
	// Called once per family (it operates on a single family).
	require.NoError(t, disallowDaemonIntrospection(false))
	require.NoError(t, disallowDaemonIntrospection(true))
	for _, useIPv6 := range []bool{false, true} {
		rules := listINPUT(t, useIPv6)
		assert.Less(t, acceptRuleIndex(rules, useIPv6), 0,
			"ACCEPT should be removed (ipv6=%v): %v", useIPv6, rules)
		assert.GreaterOrEqual(t, dropRuleIndex(rules), 0,
			"DROP must remain (ipv6=%v): %v", useIPv6, rules)
	}

	// 6. Idempotent teardown: a second Disallow removes nothing more and does not
	// error; the DROP still stands and the ACCEPT stays gone.
	require.NoError(t, disallowDaemonIntrospection(false))
	require.NoError(t, disallowDaemonIntrospection(true))
	for _, useIPv6 := range []bool{false, true} {
		rules := listINPUT(t, useIPv6)
		assert.Less(t, acceptRuleIndex(rules, useIPv6), 0,
			"ACCEPT should still be absent after second disallow (ipv6=%v): %v", useIPv6, rules)
		assert.GreaterOrEqual(t, dropRuleIndex(rules), 0,
			"DROP must remain after second disallow (ipv6=%v): %v", useIPv6, rules)
	}
}

// TestIntrospectionFirewall_IPv4Only_Integration exercises the ipv6Enabled=false
// path: every setup/allow/disallow call must touch only IPv4 and leave the IPv6
// (ip6tables) INPUT chain empty. This guards the family gating in each function,
// which the dual-stack test above never exercises (it always passes true).
func TestIntrospectionFirewall_IPv4Only_Integration(t *testing.T) {
	enterFreshNetNS(t)

	// Setup + allow with IPv6 disabled: the IPv4 rules land, the IPv6 chain stays
	// untouched.
	require.NoError(t, SetupIntrospectionFirewall(false))
	require.NoError(t, allowDaemonIntrospection(false))

	v4 := listINPUT(t, false)
	assert.GreaterOrEqual(t, dropRuleIndex(v4), 0, "IPv4 DROP should be present: %v", v4)
	assert.GreaterOrEqual(t, acceptRuleIndex(v4, false), 0, "IPv4 ACCEPT should be present: %v", v4)

	v6 := listINPUT(t, true)
	assert.Equal(t, 0, countDropRules(v6), "no IPv6 DROP should be installed: %v", v6)
	assert.Equal(t, 0, countAcceptRules(v6, true), "no IPv6 ACCEPT should be installed: %v", v6)

	// Teardown with IPv6 disabled removes the IPv4 ACCEPT and leaves the DROP; the
	// IPv6 chain remains empty throughout.
	require.NoError(t, disallowDaemonIntrospection(false))

	v4 = listINPUT(t, false)
	assert.Less(t, acceptRuleIndex(v4, false), 0, "IPv4 ACCEPT should be removed: %v", v4)
	assert.GreaterOrEqual(t, dropRuleIndex(v4), 0, "IPv4 DROP must remain: %v", v4)

	v6 = listINPUT(t, true)
	assert.Equal(t, 0, countDropRules(v6), "IPv6 chain must stay empty: %v", v6)
	assert.Equal(t, 0, countAcceptRules(v6, true), "IPv6 chain must stay empty: %v", v6)
}
