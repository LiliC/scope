package main

import (
	"context"
	"flag"

	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	cri "github.com/weaveworks/scope/runtime"

	"google.golang.org/grpc"
)

const unixProtocol = "unix"

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

func main() {
	cfg := config{}
	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// select shim via args
	flagset.StringVar(&cfg.containerRuntimeEndpoint, "container-runtime-endpoint", "unix///var/run/dockershim.sock", "The endpoint to connect to the CRI via.")
	flagset.Parse(os.Args[1:])

	addr, dailer, err := GetAddressAndDialer(cfg.containerRuntimeEndpoint)
	if err != nil {
		fmt.Printf("failed to get address and dialer: %v", err)
		os.Exit(1)
	}

	conn, err := grpc.Dial(
		addr,
		grpc.WithInsecure(),
		grpc.WithDialer(dailer),
	)
	if err != nil {
		fmt.Printf("failed to dial container runtime endpoint: %v", err)
		os.Exit(1)
	}
	defer conn.Close()

	criConn := cri.NewRuntimeServiceClient(conn)

	// list containers
	//	ctx := context.Background()
	resp, err := criConn.ListContainers(context.TODO(), &cri.ListContainersRequest{})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	for _, c := range resp.Containers {
		fmt.Printf("c: %#+v\n", c)
		fmt.Printf("imagespec.image: %#+v\n", c.Image.Image)
	}
}
