package service

import (
	"context"
	"errors"
	"net"
)

// Resolve once, validate all addresses, then dial a validated literal. Reuses
// monitor address policy and rejects non-unicast addresses as well. No proxy,
// redirect or second DNS lookup can move the request onto an internal network.
func modelEvaluationSafeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || isBlockedHostname(host) {
		return nil, errors.New("model evaluation endpoint blocked")
	}
	var addresses []net.IPAddr
	if ip := net.ParseIP(host); ip != nil {
		addresses = []net.IPAddr{{IP: ip}}
	} else {
		addresses, err = net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, errors.New("model evaluation endpoint resolution failed")
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("model evaluation endpoint resolution failed")
	}
	for _, a := range addresses {
		if isPrivateIP(a.IP) || !a.IP.IsGlobalUnicast() {
			return nil, errors.New("model evaluation endpoint blocked")
		}
	}
	for _, a := range addresses {
		conn, err := monitorDialer.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, errors.New("model evaluation endpoint connection failed")
}
