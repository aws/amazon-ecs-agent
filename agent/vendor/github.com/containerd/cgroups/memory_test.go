package cgroups

import (
	"strings"
	"testing"
)

const memoryData = `cache 1
rss 2
rss_huge 3
mapped_file 4
dirty 5
writeback 6
pgpgin 7
pgpgout 8
pgfault 9
pgmajfault 10
inactive_anon 11
active_anon 12
inactive_file 13
active_file 14
unevictable 15
hierarchical_memory_limit 16
hierarchical_memsw_limit 17
total_cache 18
total_rss 19
total_rss_huge 20
total_mapped_file 21
total_dirty 22
total_writeback 23
total_pgpgin 24
total_pgpgout 25
total_pgfault 26
total_pgmajfault 27
total_inactive_anon 28
total_active_anon 29
total_inactive_file 30
total_active_file 31
total_unevictable 32
`

func TestParseMemoryStats(t *testing.T) {
	var (
		c = &memoryController{}
		m = &MemoryStat{}
		r = strings.NewReader(memoryData)
	)
	if err := c.parseStats(r, m); err != nil {
		t.Fatal(err)
	}
	index := []uint64{
		m.Cache,
		m.RSS,
		m.RSSHuge,
		m.MappedFile,
		m.Dirty,
		m.Writeback,
		m.PgPgIn,
		m.PgPgOut,
		m.PgFault,
		m.PgMajFault,
		m.InactiveAnon,
		m.ActiveAnon,
		m.InactiveFile,
		m.ActiveFile,
		m.Unevictable,
		m.HierarchicalMemoryLimit,
		m.HierarchicalSwapLimit,
		m.TotalCache,
		m.TotalRSS,
		m.TotalRSSHuge,
		m.TotalMappedFile,
		m.TotalDirty,
		m.TotalWriteback,
		m.TotalPgPgIn,
		m.TotalPgPgOut,
		m.TotalPgFault,
		m.TotalPgMajFault,
		m.TotalInactiveAnon,
		m.TotalActiveAnon,
		m.TotalInactiveFile,
		m.TotalActiveFile,
		m.TotalUnevictable,
	}
	for i, v := range index {
		if v != uint64(i)+1 {
			t.Errorf("expected value at index %d to be %d but received %d", i, i+1, v)
		}
	}
}
