package section

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"strings"

	"k8s.io/ingress-nginx/internal/ingress"
)

// ToJSON takes the value and write it as JSON to /etc/nginx/input.json
func ToJSON(input interface{}) string {
	bin, err := json.MarshalIndent(input, "  ", "")
	if err != nil {
		return err.Error()
	}

	// www-data has permissions to write to /tmp
	err = ioutil.WriteFile("/etc/nginx/input.json", bin, 0777)
	if err != nil {
		return err.Error()
	}
	return ""
}

func fmtDomainName(name string) string {
	// If 'name' is a wildcard name, need to convert to a regex.
	// NOTE: we're only handling 1 level of wildcards system-wide at this time.
	if !strings.HasPrefix(name, "*.") {
		return name
	}
	regexName := "\"~^([a-z0-9\\-]{1,63})" + strings.ReplaceAll(name[1:], ".", "\\.") + "$\""
	return regexName
}

// reduceByAlias reduces the incoming servers by incoming Alias value, joining multiple Hostname into outgoing Alias value
func reduceByAlias(servers []*ingress.Server) []*ingress.Server {
	// The fallback server `_` has no Alias, so this relies on us using an Alias
	// for all ingress objects so there is no overlap.
	rs := map[string]ingress.Server{}
	for _, srv := range servers {
		s := *srv //shallow copy seems acceptable for this scenario
		if len(s.Aliases) == 0 && s.Hostname == "_" {
			log.Printf("[INFO] found 'non-default' ingress with no aliases (hostname: %s) (server: %+v)", s.Hostname, s)
			continue
		}
		// case: all servers except where host name isn't default '_'
		if len(s.Aliases) == 0 {
			log.Fatalf(
				"[FATAL] found ingress with no aliases (hostname: %s) (server: %+v)",
				s.Hostname, s)
		}

		alias := s.Aliases[0]
		rsa, ok := rs[alias]
		if !ok {
			s.Aliases = []string{""}
			rs[alias] = s
		} else {
			if rsa.Aliases[0] == "" {
				rsa.Aliases = []string{s.Hostname}
			} else {
				rsa.Aliases = []string{
					fmt.Sprintf("%s %s", rs[alias].Aliases[0], s.Hostname),
				}
			}
			rs[alias] = rsa
		}
	}

	srv := make([]*ingress.Server, 0)
	for _, server := range rs {
		val := server
		if strings.HasPrefix(val.Hostname, "*.") || strings.Index(val.Aliases[0], "*") > -1 {
			// Make a copy of server and convert wildcard names to regex.
			// Don't need a deep copy as we may only change Server and Alias
			val.Hostname = fmtDomainName(val.Hostname)
			aliases := strings.Split(val.Aliases[0], " ")
			for i, alias := range aliases {
				aliases[i] = fmtDomainName(alias)
			}
			val.Aliases = []string{
				strings.Join(aliases, " "),
			}
		}
		srv = append(srv, &val)
	}

	return srv
}

// ReduceByAliasNoop doesn't perform the reduce of incoming servers by
// incoming Alias value.
func ReduceByAliasNoop(servers []*ingress.Server) []*ingress.Server {
	return servers
}

// reduceByAlias reduces the incoming servers by incoming Alias value,
// joining multiple Hostname into outgoing Alias value
func ReduceByAlias(servers []*ingress.Server) []*ingress.Server {
	return reduceByAlias(servers)
}

func Provider(label string) string {
	return fmt.Sprintf("%s: %s", label, "section.io (v0.46)")
}
