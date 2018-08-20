package cri

import (
	"context"
	"fmt"

	client "github.com/weaveworks/scope/cri/runtime"
	"github.com/weaveworks/scope/probe/docker"
	"github.com/weaveworks/scope/report"
)

// Reporter generate Reports containing Container and ContainerImage topologies
type Reporter struct {
	cri    client.RuntimeServiceClient
	hostID string
}

// NewReporter makes a new Reporter
func NewReporter(cri client.RuntimeServiceClient) *Reporter {
	reporter := &Reporter{
		cri:    cri,
		hostID: "minikube",
	}

	return reporter
}

// Name of this reporter, for metrics gathering
func (Reporter) Name() string { return "CRI" }

// Report generates a Report containing Container topologies
func (r *Reporter) Report() (report.Report, error) {
	result := report.MakeReport()
	containerTopol, err := r.containerTopology()
	if err != nil {
		return report.MakeReport(), err
	}

	result.Container = result.Container.Merge(containerTopol)
	return result, nil
}

func (r *Reporter) getIPs(c *client.Container) ([]string, error) {
	ips := []string{}

	status, err := r.cri.PodSandboxStatus(context.TODO(), &client.PodSandboxStatusRequest{PodSandboxId: c.PodSandboxId, Verbose: true})
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	s := status.Status
	ips = append(ips, s.Network.Ip)
	fmt.Println("ips:")
	fmt.Println(ips)

	return ips, nil
}

func (r *Reporter) containerTopology() (report.Topology, error) {
	result := report.MakeTopology().
		WithMetadataTemplates(docker.ContainerImageMetadataTemplates).
		WithTableTemplates(docker.ContainerImageTableTemplates)

	ctx := context.Background()
	resp, err := r.cri.ListContainers(ctx, &client.ListContainersRequest{})
	if err != nil {
		return result, err
	}

	nodes := []report.Node{}
	hostNetworkInfo := report.MakeSets()
	for _, c := range resp.Containers {
		node := getNode(c)
		if hostIPs, err := r.getIPs(c); err == nil {
			// TODO: save hostID/nodeID?
			hostIPsWithScopes := addScopeToIPs(r.hostID, hostIPs)
			hostNetworkInfo = hostNetworkInfo.
				Add(docker.ContainerIPs, report.MakeStringSet(hostIPs...)).
				Add(docker.ContainerIPsWithScopes, report.MakeStringSet(hostIPsWithScopes...))
		}
		node = node.WithSets(hostNetworkInfo)
		nodes = append(nodes, node)
	}
	// Network info

	for _, node := range nodes {
		result.AddNode(node)
	}

	fmt.Println(result)
	fmt.Printf("%#+v\n", result)

	return result, nil
}

func addScopeToIPs(hostID string, ips []string) []string {
	ipsWithScopes := []string{}
	for _, ip := range ips {
		ipsWithScopes = append(ipsWithScopes, report.MakeAddressNodeID(hostID, ip))
	}
	return ipsWithScopes
}

func getNode(c *client.Container) report.Node {
	result := report.MakeNodeWith(report.MakeContainerNodeID(c.Id), map[string]string{
		docker.ContainerName:         c.Metadata.Name,
		docker.ContainerID:           c.Id,
		docker.ContainerState:        fmt.Sprintf("%v", c.State),
		docker.ContainerRestartCount: fmt.Sprintf("%v", c.Metadata.Attempt),
		docker.ImageID:               c.ImageRef,
		docker.ImageName:             c.Image.Image,
	}).WithParents(report.MakeSets().
		Add(report.ContainerImage, report.MakeStringSet(report.MakeContainerImageNodeID(c.ImageRef))),
	)
	result = result.AddPrefixPropertyList(docker.LabelPrefix, c.Labels)

	return result
}
