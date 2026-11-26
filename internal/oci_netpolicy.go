package internal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// A public registry must not be able to make Dockan connect to the user's
// internal network (SSRF): the token server it names in its Bearer challenge
// and the hosts it redirects blob downloads to must be public HTTPS
// endpoints. A registry the user targets on a local or private address is
// trusted with private addresses too.

type networkPolicyKey struct{}

// withNetworkPolicy marks a request context as allowed (or not) to reach
// private and loopback addresses.
func withNetworkPolicy(ctx context.Context, allowPrivate bool) context.Context {
	return context.WithValue(ctx, networkPolicyKey{}, allowPrivate)
}

func privateAllowed(ctx context.Context) bool {
	allow, ok := ctx.Value(networkPolicyKey{}).(bool)
	return ok && allow
}

// nonPublicNets lists special-purpose ranges that the standard library does
// not classify as private but that must never be reached on behalf of a
// public registry (RFC 6890 and related).
var nonPublicNets = mustCIDRs(
	"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
	"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
	"::/96", "100::/64", "2001:db8::/32", "fec0::/10",
)

func mustCIDRs(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

var (
	nat64Prefix = mustCIDRs("64:ff9b::/96")[0]
	sixToFour   = mustCIDRs("2002::/16")[0]
)

// isPublicIP reports whether ip is a globally routable unicast address.
// IPv6 forms that embed an IPv4 address (IPv4-mapped, NAT64, 6to4) are
// judged by that IPv4 address.
func isPublicIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	} else if len(ip) == net.IPv6len && nat64Prefix.Contains(ip) {
		return isPublicIP(net.IP(ip[12:16]))
	} else if len(ip) == net.IPv6len && sixToFour.Contains(ip) {
		return isPublicIP(net.IP(ip[2:6]))
	}
	for _, n := range nonPublicNets {
		if n.Contains(ip) {
			return false
		}
	}
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.Equal(net.IPv4bcast))
}

func hostOnly(hostport string) string {
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(hostport, "[]")
}

// privateRegistriesEnv lists registry host names the user trusts with
// private addresses, for registries reached by name on the local network
// (comma-separated, for example "registry.lan,nas.home:5000").
const privateRegistriesEnv = "DOCKAN_PRIVATE_REGISTRIES"

// registryIsLocal reports whether the registry the user asked for is local:
// an IP literal on a non-public address, localhost, or a name listed in
// DOCKAN_PRIVATE_REGISTRIES. Names are never judged by DNS: a public registry
// controls its DNS and could answer once with a private address to lift the
// protection.
func registryIsLocal(hostport string) bool {
	host := strings.ToLower(hostOnly(hostport))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return !isPublicIP(ip)
	}
	for _, entry := range strings.Split(os.Getenv(privateRegistriesEnv), ",") {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry != "" && (entry == host || entry == strings.ToLower(hostport)) {
			return true
		}
	}
	return false
}

// checkOutboundURL rejects a URL a public registry led Dockan to, when it is
// not HTTPS or names an internal address.
func checkOutboundURL(ctx context.Context, u *url.URL) error {
	if privateAllowed(ctx) {
		return nil
	}
	if u.Scheme != "https" {
		return fmt.Errorf("adresse refusée (HTTPS requis): %s", u.Redacted())
	}
	host := hostOnly(u.Host)
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return fmt.Errorf("adresse interne refusée: %s", u.Redacted())
		}
		return nil
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return fmt.Errorf("adresse interne refusée: %s", u.Redacted())
	}
	return nil // names are checked again on the resolved address at dial time
}

var errInternalAddress = errors.New("adresse interne refusée")

// isConfiguredProxy reports whether addr is the proxy configured for HTTP or
// HTTPS (the variables Go's transport honours). Only a connection to the
// proxy itself skips the dial-time check: the proxy often sits on a private
// address. Hosts reached directly (no proxy, or listed in NO_PROXY) are
// always checked. Behind a proxy, the proxy resolves names itself: only the
// URL-level check applies to them.
func isConfiguredProxy(addr string) bool {
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		raw := os.Getenv(key)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "://") {
			raw = "http://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			continue
		}
		port := u.Port()
		if port == "" {
			port = map[string]string{"https": "443", "socks5": "1080"}[u.Scheme]
			if port == "" {
				port = "80"
			}
		}
		if strings.EqualFold(net.JoinHostPort(u.Hostname(), port), addr) {
			return true
		}
	}
	return false
}

// policyDialContext resolves the target itself and refuses internal
// addresses for public-only requests, so a name cannot switch to an internal
// address between the URL check and the connection (DNS rebinding).
func policyDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if privateAllowed(ctx) || isConfiguredProxy(addr) {
			return dialer.DialContext(ctx, network, addr)
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		var lastErr error = fmt.Errorf("%w: %s", errInternalAddress, host)
		for _, ip := range ips {
			if !isPublicIP(ip) {
				continue
			}
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

func newOCIHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = policyDialContext(&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second})
	return &http.Client{
		Timeout:   600 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("trop de redirections")
			}
			return checkOutboundURL(req.Context(), req.URL)
		},
	}
}
