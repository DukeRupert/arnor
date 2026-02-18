package caddy

import "fmt"

// Generate returns a Caddyfile site block that reverse-proxies to the given port.
// For production domains (not subdomains of angmar.dev), it includes a www redirect.
func Generate(domain string, port int) string {
	block := fmt.Sprintf(`%s {
	reverse_proxy localhost:%d
}
`, domain, port)

	// Add www redirect for prod domains (not dev subdomains)
	if !isDevDomain(domain) {
		block += fmt.Sprintf(`
www.%s {
	redir https://%s{uri} permanent
}
`, domain, domain)
	}

	return block
}

func isDevDomain(domain string) bool {
	// Dev domains are subdomains of angmar.dev
	return len(domain) > len("angmar.dev") &&
		domain[len(domain)-len("angmar.dev"):] == "angmar.dev"
}
