package cri

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"google.golang.org/grpc"

	criClient "github.com/weaveworks/scope/runtime"
)

const unixProtocol = "unix"

// TODO: criClient.RuntimeServiceClient
// Client interface for mocking.

type config struct {
	containerRuntimeEndpoint string
}

func dial(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout(unixProtocol, addr, timeout)
}

func GetAddressAndDialer(endpoint string) (string, func(addr string, timeout time.Duration) (net.Conn, error), error) {
	protocol, addr, err := parseEndpointWithFallbackProtocol(endpoint, unixProtocol)
	if err != nil {
		return "", nil, err
	}
	if protocol != unixProtocol {
		return "", nil, fmt.Errorf("only support unix socket endpoint")
	}

	return addr, dial, nil
}

func parseEndpointWithFallbackProtocol(endpoint string, fallbackProtocol string) (protocol string, addr string, err error) {
	if protocol, addr, err = parseEndpoint(endpoint); err != nil && protocol == "" {
		fallbackEndpoint := fallbackProtocol + "://" + endpoint
		protocol, addr, err = parseEndpoint(fallbackEndpoint)
		if err == nil {
			fmt.Println(err)
		}
	}
	return
}

func parseEndpoint(endpoint string) (string, string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", "", err
	}

	if u.Scheme == "tcp" {
		return "tcp", u.Host, nil
	} else if u.Scheme == "unix" {
		return "unix", u.Path, nil
	} else if u.Scheme == "" {
		return "", "", fmt.Errorf("Using %q as endpoint is deprecated, please consider using full url format", endpoint)
	} else {
		return u.Scheme, "", fmt.Errorf("protocol %q not supported", u.Scheme)
	}
}

func NewCRIClient(endpoint string) (criClient.RuntimeServiceClient, error) {
	if endpoint == "" {
		// We default to docker endpoint
		endpoint = "unix///var/run/dockershim.sock"
	}

	// Dial grpc endpoint
	addr, dailer, err := GetAddressAndDialer(endpoint)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithDialer(dailer))
	if err != nil {
		return nil, err
	}

	return criClient.NewRuntimeServiceClient(conn), nil
}
