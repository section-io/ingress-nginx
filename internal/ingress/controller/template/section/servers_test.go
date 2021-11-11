package section

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const defaultTestDataDir = "testdata"

func DataFile(name string) string {
	dir := os.Getenv("TEST_INGRESS_SERVERS_DATA")
	if dir == "" {
		dir = defaultTestDataDir
	}
	return filepath.Join(dir, name)
}

func Test_Reduce_ShouldHaveAllServers(t *testing.T) {
	cases := []struct {
		msg      string
		filename string
	}{
		{
			filename: "0-input.json",
		},
	}
	for _, c := range cases {
		servers := MustLoadServers(DataFile(c.filename))
		assert.NotNil(t, servers)

		hostnames := servers.Hostnames()
		assert.Equal(t, servers.Len(), len(hostnames))

		grouped := groupByAlias(servers)
		assert.Equal(t, len(hostnames), len(grouped.Hostnames()))

		for _, name := range hostnames {
			assert.True(t, grouped.ContainsHostname(name), "missing hostname: %s", name)
		}
	}
}

func Test_Reduce_ShouldProduceCorrectServers(t *testing.T) {
	cases := []struct {
		msg       string
		filename  string
		hostnames []string
	}{
		{
			filename: "0-input.json",
			hostnames: []string{
				"bacon.section.io",
				"redirector.mysection.io",
				"_",
			},
		},
	}
	for _, c := range cases {
		before := MustLoadServers(DataFile(c.filename))
		assert.True(t, before.ContainsHostname("_"))
		after := groupByAlias(before)
		assert.True(t, len(after) <= len(before))
		for _, h := range c.hostnames {
			assert.True(t, after.ContainsHostname(h))
		}
	}
}

func Test_AllServerFile_ShouldContainerDefaultHost(t *testing.T) {
	filenames := []string{
		"0-input.json",
	}
	hostnames := []string{
		"_",
	}
	for _, filename := range filenames {
		servers := MustLoadServers(DataFile(filename))
		assert.True(t, len(servers) > 0)
		for _, h := range hostnames {
			assert.True(t, servers.ContainsHostname(h),
				"failed to find defualt host in (filename: %s)", filename,
			)
		}
	}
}

func Test_AllServerFiles_ShouldContainTheseHosts(t *testing.T) {
	filenames := []string{
		"0-input.json",
	}
	hostnames := []string{
		"bacon.section.io",
		"redirector.mysection.io",
		"_",
	}
	for _, filename := range filenames {
		servers := MustLoadServers(DataFile(filename))
		assert.True(t, len(servers) > 0)
		for _, h := range hostnames {
			assert.True(t, servers.ContainsHostname(h))
		}
	}
}

func Test_MustLoadServer(t *testing.T) {
	cases := []struct {
		msg       string
		filename  string
		hostnames []string
	}{
		{
			filename: "0-input.json",
			hostnames: []string{
				"bacon.section.io",
				"redirector.mysection.io",
			},
		},
	}
	for _, c := range cases {
		servers := MustLoadServers(DataFile(c.filename))
		assert.True(t, len(servers) > 0)
		for _, h := range c.hostnames {
			assert.True(t, servers.ContainsHostname(h))
		}
	}
}
