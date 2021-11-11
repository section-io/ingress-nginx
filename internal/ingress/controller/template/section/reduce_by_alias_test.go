package section

import (
	"testing"

	"k8s.io/ingress-nginx/internal/ingress"
)

func quickServer(hostname, alias string) *ingress.Server {
	return &ingress.Server{
		Hostname: hostname,
		// A single alias is proivded, even though nginx-ingress-objects
		// can provide multiple aliases.  Those aliases result in multiple
		// server blocks templated out in the nginx.conf file.  Lot's of
		// server blocks consume a high amount of memory.  Since we host
		// 10k+ domains, this won't work for our use case.

		// However, we've hijacked aliases to be the identifier by which
		// to group servers to reduce mulitple aliases into a single
		// server block which ultimately
		Aliases: []string{alias},
	}
}

func TestReduceByAlias_Grouping(t *testing.T) {
	cases := []struct {
		in    []*ingress.Server
		count int
		msg   string
		skip  bool
	}{
		{
			in: []*ingress.Server{
				quickServer("host1", "1"),
				quickServer("host5", "2"),
			},
			count: 2,
			msg:   "expected %d servers even though we haven't constructed 'groups': %d",
		},
		{
			in: []*ingress.Server{
				quickServer("host1", "1"),
				quickServer("host5", "2"),
				quickServer("host9", "3"),
			},
			count: 3,
			msg:   "expected %d servers even though we haven't constructed 'groups': %d",
		},
		{
			in: []*ingress.Server{
				// group 1
				quickServer("host1", "123"), quickServer("host2", "123"),
				quickServer("host3", "123"), quickServer("host4", "123"),

				// group 2
				quickServer("host5", "yyy"), quickServer("host6", "yyy"),

				// group 3
				quickServer("host7", "aaa"), quickServer("host8", "aaa"),
				quickServer("host9", "aaa"),
			},
			count: 3,
			msg:   "expected %d servers since there are 3 alias groups, but got: %d",
		},
		{
			in: []*ingress.Server{
				// group 1
				quickServer("host1", "123"), quickServer("host2", "123"),
				quickServer("host3", "123"), quickServer("host4", "123"),

				// group 2
				&ingress.Server{
					Aliases:  []string{},
					Hostname: "n1.no.aliases",
				},

				// group 3
				&ingress.Server{
					Aliases:  []string{},
					Hostname: "n2.no.aliases",
				},

				// group 4
				quickServer("host7", "aaa"), quickServer("host8", "aaa"),
				quickServer("host9", "aaa"),
			},
			count: 4,
			// need to verify that EVERY ingress-object will have an alias
			msg:  "handles zero aliases in a server, groups: (expected:%d, actual:%d)",
			skip: true,
		},
	}
	for _, c := range cases {
		if c.skip {
			continue
		}
		out := ReduceByAlias(c.in)
		if len(out) != c.count {
			t.Errorf(c.msg, c.count, len(out))
		}
	}

}

func TestReduceByAlias_TrivialCases(t *testing.T) {
	cases := []struct {
		in    []*ingress.Server
		count int
		msg   string
	}{
		{
			in:    []*ingress.Server{},
			count: 0,
			msg:   "expected empty list of servers (empty list)",
		},
		{
			in:    nil,
			count: 0,
			msg:   "expected empty list of servers (nil list)",
		},
		{
			in: []*ingress.Server{
				&ingress.Server{Aliases: []string{"_"}, Hostname: "default"},
			},
			count: 1,
			msg:   "expected 1 servers (barely a use case)",
		},
	}
	for _, c := range cases {
		out := ReduceByAlias(c.in)
		if len(out) != c.count {
			t.Errorf(c.msg)
		}
	}

}
