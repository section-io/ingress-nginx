package section

import (
	"log"
	"strings"

	"k8s.io/ingress-nginx/internal/ingress"
)

// groupByAlias reduces the incoming servers by incoming Alias value,
// joining multiple Hostname into outgoing Alias value
func groupByAlias(servers Servers) AliasGroup {
	// The fallback server `_` has no Alias, so this relies on us using an Alias
	// for all ingress objects so there is no overlap.
	rs := map[string]ingress.Server{}
	for _, srv := range servers {
		s := *srv //shallow copy seems acceptable for this scenario
		if len(s.Aliases) == 0 && s.Hostname == "_" {
			log.Printf("[INFO] found 'non-default' ingress with no aliases (hostname: %s) (server: %+v)", s.Hostname, s)
			s.Aliases = []string{"default-server-via-underscore"}
		}
		// case: all servers except where host name isn't default '_'
		if len(s.Aliases) == 0 {
			log.Fatalf(
				"[FATAL] found ingress with no aliases (hostname: %s) (server: %+v)",
				s.Hostname, s)
		}
		// environment provisioner makes all ingress objects with one
		// alias, so likely this is simply returning s.Aliases[0]
		aliasKey := strings.Join(s.Aliases, "")
		rsa, ok := rs[aliasKey]
		if !ok {
			s.Aliases = []string{}
			rs[aliasKey] = s
		} else {
			groupKey := strings.Join(rsa.Aliases, "")
			if groupKey == "" {
				rsa.Aliases = []string{s.Hostname}
			} else {
				rsa.Aliases = append(rsa.Aliases, s.Hostname)
			}
			// re-assigning entry for this key with the updated STRUCT
			rs[aliasKey] = rsa
		}
	}
	return rs
}

func convertWildCardsToRegex(rs map[string]ingress.Server) Servers {
	srv := make([]*ingress.Server, 0)
	for _, server := range rs {
		val := server
		joinedAliases := strings.Join(val.Aliases, " ")
		hostnameHasWildcard := strings.HasPrefix(val.Hostname, "*.")
		aliasesHasWildcard := strings.Index(joinedAliases, "*") > -1
		if hostnameHasWildcard || aliasesHasWildcard {
			// Make a copy of server and convert wildcard names to regex.
			// Don't need a deep copy as we may only change Server and Alias
			val.Hostname = fmtDomainName(val.Hostname)
			aliases := make([]string, len(val.Aliases))
			for i, alias := range val.Aliases {
				aliases[i] = fmtDomainName(alias)
			}
			val.Aliases = aliases
		}
		srv = append(srv, &val)
	}
	return srv
}
