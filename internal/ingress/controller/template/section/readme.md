# Purpose

These files are specific to features we need to add for our internal
version of the ingress-nginx image/repo.

1. We need to reduce the number of server blocks.  Normally all
   aliases are templated one to a server block.  This causes too many
   server blocks which consumes too much nginx memory in cases where
   there are thousands of server blocks (jxt, realpage, etc)

2. We need to know when certs are no longer cached or evicted from
   internal caches so that we can properly diagnose when customers are
   served the wrong certs.  We've added metrics for OPs to the Lua and
   surfaced via the ingress-nginx controller.

3. testdata/ holds an example of the data passed to the template
   (v0.46) which is useful when considering template changes.  These
   file(s) can be used to test the grouping servers (possibly by
   alias).
