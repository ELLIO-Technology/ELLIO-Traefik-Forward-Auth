package ipmatcher

import (
	"net/netip"
	"sync/atomic"

	"go4.org/netipx"
)

// Matcher provides thread-safe IP address matching against an IP set
type Matcher struct {
	ipset atomic.Value // stores *netipx.IPSet
	count atomic.Int64
}

// New creates a new IP matcher
func New() *Matcher {
	m := &Matcher{}
	// Initialize with empty IPSet
	empty, _ := (&netipx.IPSetBuilder{}).IPSet()
	m.ipset.Store(empty)
	return m
}

// Contains checks if the given IP address is in the set
func (m *Matcher) Contains(ip netip.Addr) bool {
	set := m.ipset.Load().(*netipx.IPSet)
	return set.Contains(ip)
}

// Update atomically replaces the IP set with a new one
func (m *Matcher) Update(ipset *netipx.IPSet, count int64) {
	m.ipset.Store(ipset)
	m.count.Store(count)
}

// Count returns the number of entries in the current IP set
func (m *Matcher) Count() int64 {
	return m.count.Load()
}