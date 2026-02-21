package caddy

import "fmt"

// Generate returns a Caddyfile site block that reverse-proxies to the given port.
// For production domains (not subdomains of angmar.dev), it includes a www redirect.
// When dnsProvider is "cloudflare", a tls block is added so Caddy uses the
// ACME DNS-01 challenge via the caddy-dns/cloudflare module.
func Generate(domain string, port int, dnsProvider string) string {
	tls := tlsBlock(dnsProvider)

	block := fmt.Sprintf(`%s {%s
	reverse_proxy localhost:%d
}
`, domain, tls, port)

	// Add www redirect for prod domains (not dev subdomains)
	if !isDevDomain(domain) {
		block += fmt.Sprintf(`
www.%s {%s
	redir https://%s{uri} permanent
}
`, domain, tls, domain)
	}

	return block
}

func tlsBlock(dnsProvider string) string {
	if dnsProvider == "cloudflare" {
		return "\n\ttls {\n\t\tdns cloudflare {env.CF_API_TOKEN}\n\t}"
	}
	return ""
}

func isDevDomain(domain string) bool {
	// Dev domains are subdomains of angmar.dev
	return len(domain) > len("angmar.dev") &&
		domain[len(domain)-len("angmar.dev"):] == "angmar.dev"
}
