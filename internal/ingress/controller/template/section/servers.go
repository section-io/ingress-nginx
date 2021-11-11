package section

import (
	"encoding/json"
	"io/ioutil"
	"sort"

	"k8s.io/ingress-nginx/internal/ingress"
)

// MustLoadServers reads the given file and unmarshals the JSON as
// Servers (a slice of *Server)
func MustLoadServers(filename string) Servers {
	bin, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	servers := []*ingress.Server{}
	err = json.Unmarshal(bin, &servers)
	if err != nil {
		panic(err)
	}
	sort.Sort(SortServersByHostName(servers))
	return servers
}

// AliasGroup is the output of GroupByAlias which we use in the
// template.  This type is also useful for querying during tests to
// gaurantee invariants.
type AliasGroup map[string]ingress.Server

func (a AliasGroup) ContainsHostname(hostname string) bool {
	for _, server := range a {
		if server.Hostname == hostname {
			return true
		}
		for _, alias := range server.Aliases {
			if alias == hostname {
				return true
			}
		}
	}
	return false
}

// Hostnames returns all the hostnames and aliases for all the servers
// in the collection
func (a AliasGroup) Hostnames() []string {
	names := []string{}
	for _, server := range a {
		names = append(names, server.Hostname)
		for _, alias := range server.Aliases {
			names = append(names, alias)
		}
	}
	return names
}

// Servers is a slice of *Server which can be queries in tests and is
// the internal result after grouping and for which the nginx.template
// is generally expecting
type Servers []*ingress.Server

// Len provides the number of servers
func (servers Servers) Len() int {
	return len(servers)
}

// ContainsHostname returns true if the given hostname is contained in
// the collection
func (servers Servers) ContainsHostname(hostname string) bool {
	for _, s := range servers {
		if s.Hostname == hostname {
			return true
		}
	}
	return false
}

// Hostnames produces a list of all the hostnames for all the servers
// in the collection
func (servers Servers) Hostnames() []string {
	names := make([]string, len(servers))
	for i, s := range servers {
		names[i] = s.Hostname
	}
	return names
}

// SortServersByHostName is a convenience type for producing more
// regular and well organized template output or visually inspecting a
// rendered nginx.tmpl file, and for comparing lists in tests
type SortServersByHostName Servers

// Len returns the number of *Servers in the collection
func (s SortServersByHostName) Len() int {
	return len(s)
}

// Less compares the Hostnames at the given indices
func (s SortServersByHostName) Less(i, j int) bool {
	return s[i].Hostname < s[j].Hostname
}

// Swap exchanges the servers at the given indices.
func (s SortServersByHostName) Swap(i, j int) {
	a := s[i]
	b := s[j]
	s[i] = b
	s[j] = a
}
