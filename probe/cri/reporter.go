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
		cri: cri,
		// TODO: get host ID from CRI?
		hostID: "minikube",
	}

	return reporter
}

// Name of this reporter, for metrics gathering
func (Reporter) Name() string { return "CRI" }

// Report generates a Report containing Container topologies
func (r *Reporter) Report() (report.Report, error) {
	result := report.MakeReport()
	cImageTopology, err := r.containerImageTopology()
	if err != nil {
		return report.MakeReport(), err
	}

	cTopology, err := r.containerTopology()
	if err != nil {
		return report.MakeReport(), err
	}

	result.Container = result.ContainerImage.Merge(cImageTopology)
	result.Container = result.Container.Merge(cTopology)
	//result.Overlay = result.Overlay.Merge(r.overlayTopology())

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

// This gets us the basic image information.
func (r *Reporter) containerImageTopology() (report.Topology, error) {
	result := report.MakeTopology().
		WithMetadataTemplates(docker.ContainerImageMetadataTemplates).
		WithTableTemplates(docker.ContainerImageTableTemplates)

	ctx := context.Background()
	resp, err := r.cri.ListContainers(ctx, &client.ListContainersRequest{})
	if err != nil {
		return result, err
	}

	for _, c := range resp.Containers {
		latests := map[string]string{
			docker.ImageID:   c.ImageRef,
			docker.ImageName: c.Image.Image,
		}
		nodeID := report.MakeContainerImageNodeID(c.ImageRef)
		node := report.MakeNodeWith(nodeID, latests)
		result.AddNode(node)
	}

	return result, nil
}

// This gets us container information
func (r *Reporter) containerTopology() (report.Topology, error) {
	result := report.MakeTopology().
		WithMetadataTemplates(docker.ContainerMetadataTemplates).
		WithMetricTemplates(docker.ContainerMetricTemplates).
		WithTableTemplates(docker.ContainerTableTemplates)
	result.Controls.AddControls(docker.ContainerControls)

	ctx := context.Background()
	resp, err := r.cri.ListContainers(ctx, &client.ListContainersRequest{})
	if err != nil {
		return result, err
	}

	hostNetworkInfo := report.MakeSets()
	for _, c := range resp.Containers {
		if hostIPs, err := r.getIPs(c); err == nil {
			hostIPsWithScopes := addScopeToIPs(r.hostID, hostIPs)
			hostNetworkInfo = hostNetworkInfo.
				Add(docker.ContainerIPs, report.MakeStringSet(hostIPs...)).
				Add(docker.ContainerIPsWithScopes, report.MakeStringSet(hostIPsWithScopes...))
		}
		latests := map[string]string{
			docker.ContainerName:         c.Metadata.Name,
			docker.ContainerID:           c.Id,
			docker.ContainerState:        fmt.Sprintf("%v", c.State),
			docker.ContainerRestartCount: fmt.Sprintf("%v", c.Metadata.Attempt),
		}
		nodeID := report.MakeContainerImageNodeID(c.ImageRef)
		node := report.MakeNodeWith(nodeID, latests)
		node = node.WithSets(hostNetworkInfo)
		result.AddNode(node)
	}

	fmt.Println(result)
	fmt.Printf("%#+v\n", result)

	return result, nil
}

// This should get us the overlay edges:
// Overlay nodes are active peers in any software-defined network that's overlaid on the infrastructure.
// The information is scraped by polling their status endpoints. Edges are present.
/*
func (r *Reporter) overlayTopology() report.Topology {
	// TODO: get subnets?
}
*/

func addScopeToIPs(hostID string, ips []string) []string {
	ipsWithScopes := []string{}
	for _, ip := range ips {
		ipsWithScopes = append(ipsWithScopes, report.MakeAddressNodeID(hostID, ip))
	}
	return ipsWithScopes
}
