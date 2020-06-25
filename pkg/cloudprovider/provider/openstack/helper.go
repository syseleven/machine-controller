/*
Copyright 2019 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package openstack

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	osavailabilityzones "github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/availabilityzones"
	osservergroups "github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/servergroups"
	osflavors "github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	osimages "github.com/gophercloud/gophercloud/openstack/compute/v2/images"
	osregions "github.com/gophercloud/gophercloud/openstack/identity/v3/regions"
	osfloatingips "github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	ossecuritygroups "github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/security/groups"
	osecruritygrouprules "github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/security/rules"
	osnetworks "github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	osports "github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	ossubnets "github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
	"github.com/gophercloud/gophercloud/pagination"
)

var (
	errNotFound               = errors.New("not found")
	securityGroupCreationLock = sync.Mutex{}
)

const (
	openstackFloatingIPErrorStatusName = "ERROR"

	floatingReassignIPCheckPeriod = 3 * time.Second
)

func getRegion(client *gophercloud.ProviderClient, name string) (*osregions.Region, error) {
	idClient, err := goopenstack.NewIdentityV3(client, gophercloud.EndpointOpts{})
	if err != nil {
		return nil, err
	}

	return osregions.Get(idClient, name).Extract()
}

func getRegions(client *gophercloud.ProviderClient) ([]osregions.Region, error) {
	idClient, err := goopenstack.NewIdentityV3(client, gophercloud.EndpointOpts{})
	if err != nil {
		return nil, err
	}

	listOpts := osregions.ListOpts{
		ParentRegionID: "",
	}
	allPages, err := osregions.List(idClient, listOpts).AllPages()
	if err != nil {
		return nil, err
	}
	regions, err := osregions.ExtractRegions(allPages)
	if err != nil {
		return nil, err
	}
	return regions, nil
}

func getAvailabilityZones(client *gophercloud.ProviderClient, region string) ([]osavailabilityzones.AvailabilityZone, error) {
	computeClient, err := goopenstack.NewComputeV2(client, gophercloud.EndpointOpts{Region: region})
	if err != nil {
		return nil, err
	}

	allPages, err := osavailabilityzones.List(computeClient).AllPages()
	if err != nil {
		return nil, err
	}
	return osavailabilityzones.ExtractAvailabilityZones(allPages)
}

func getAvailabilityZone(client *gophercloud.ProviderClient, region, name string) (*osavailabilityzones.AvailabilityZone, error) {
	zones, err := getAvailabilityZones(client, region)
	if err != nil {
		return nil, err
	}

	for _, z := range zones {
		if z.ZoneName == name {
			return &z, nil
		}
	}

	return nil, errNotFound
}

func getImageByName(client *gophercloud.ProviderClient, region, name string) (*osimages.Image, error) {
	computeClient, err := goopenstack.NewComputeV2(client, gophercloud.EndpointOpts{Region: region})
	if err != nil {
		return nil, err
	}

	var allImages []osimages.Image
	pager := osimages.ListDetail(computeClient, osimages.ListOpts{Name: name})
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		images, err := osimages.ExtractImages(page)
		if err != nil {
			return false, err
		}
		allImages = append(allImages, images...)
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	if len(allImages) == 0 {
		return nil, errNotFound
	}
	return &allImages[0], nil
}

func getFlavor(client *gophercloud.ProviderClient, region, name string) (*osflavors.Flavor, error) {
	computeClient, err := goopenstack.NewComputeV2(client, gophercloud.EndpointOpts{Region: region})
	if err != nil {
		return nil, err
	}

	var allFlavors []osflavors.Flavor

	pager := osflavors.ListDetail(computeClient, osflavors.ListOpts{})
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		flavors, err := osflavors.ExtractFlavors(page)
		if err != nil {
			return false, err
		}
		allFlavors = append(allFlavors, flavors...)
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	for _, f := range allFlavors {
		if f.Name == name {
			return &f, nil
		}
	}

	return nil, errNotFound
}

func getSecurityGroup(client *gophercloud.ProviderClient, region, name string) (*ossecuritygroups.SecGroup, error) {
	netClient, err := goopenstack.NewNetworkV2(client, gophercloud.EndpointOpts{Region: region})
	if err != nil {
		return nil, err
	}

	var allGroups []ossecuritygroups.SecGroup
	pager := ossecuritygroups.List(netClient, ossecuritygroups.ListOpts{})
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		groups, err := ossecuritygroups.ExtractGroups(page)
		if err != nil {
			return false, err
		}
		allGroups = append(allGroups, groups...)
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	for _, g := range allGroups {
		if g.Name == name {
			return &g, nil
		}
	}

	return nil, errNotFound
}

func serverGroupByName(computeClient *gophercloud.ServiceClient, n string) (*osservergroups.ServerGroup, error) {
	var sg *osservergroups.ServerGroup
	err := osservergroups.List(computeClient).EachPage(func(page pagination.Page) (bool, error) {
		items, err := osservergroups.ExtractServerGroups(page)
		if err != nil {
			return false, err
		}
		for _, item := range items {
			if item.Name == n {
				sg = &item
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if sg == nil {
		return nil, errNotFound
	}
	return sg, nil
}

func getNetworks(client *gophercloud.ProviderClient, region string) ([]osnetworks.Network, error) {
	netClient, err := goopenstack.NewNetworkV2(client, gophercloud.EndpointOpts{Region: region})
	if err != nil {
		return nil, err
	}

	var allNetworks []osnetworks.Network
	pager := osnetworks.List(netClient, osnetworks.ListOpts{})
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		networks, err := osnetworks.ExtractNetworks(page)
		if err != nil {
			return false, err
		}
		allNetworks = append(allNetworks, networks...)
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	return allNetworks, nil
}

func getNetwork(client *gophercloud.ProviderClient, region, nameOrID string) (*osnetworks.Network, error) {
	allNetworks, err := getNetworks(client, region)
	if err != nil {
		return nil, err
	}

	for _, n := range allNetworks {
		if n.Name == nameOrID || n.ID == nameOrID {
			return &n, nil
		}
	}

	return nil, errNotFound
}

func getSubnets(client *gophercloud.ProviderClient, region, networkID string) ([]ossubnets.Subnet, error) {
	netClient, err := goopenstack.NewNetworkV2(client, gophercloud.EndpointOpts{Region: region})
	if err != nil {
		return nil, err
	}

	listOpts := ossubnets.ListOpts{}
	if networkID != "" {
		listOpts = ossubnets.ListOpts{NetworkID: networkID}
	}
	var allSubnets []ossubnets.Subnet
	pager := ossubnets.List(netClient, listOpts)
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		subnets, err := ossubnets.ExtractSubnets(page)
		if err != nil {
			return false, err
		}
		allSubnets = append(allSubnets, subnets...)
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return allSubnets, nil
}

func getSubnet(client *gophercloud.ProviderClient, region, nameOrID string) (*ossubnets.Subnet, error) {
	allSubnets, err := getSubnets(client, region, "")
	if err != nil {
		return nil, err
	}
	for _, s := range allSubnets {
		if s.Name == nameOrID || s.ID == nameOrID {
			return &s, nil
		}
	}

	return nil, errNotFound
}

func ensureKubernetesSecurityGroupExist(client *gophercloud.ProviderClient, region, name string) error {
	// We need a mutex here because otherwise if more than one machine gets created at roughly the same time
	// we will create two security groups and subsequently not be able anymore to identify our security group
	// by name
	securityGroupCreationLock.Lock()
	defer securityGroupCreationLock.Unlock()

	netClient, err := goopenstack.NewNetworkV2(client, gophercloud.EndpointOpts{Region: region})
	if err != nil {
		return osErrorToTerminalError(err, "failed to get network client")
	}

	_, err = getSecurityGroup(client, region, name)
	if err != nil {
		if err == errNotFound {
			sg, err := ossecuritygroups.Create(netClient, ossecuritygroups.CreateOpts{Name: name}).Extract()
			if err != nil {
				return osErrorToTerminalError(err, fmt.Sprintf("failed to create security group %s", name))
			}

			rules := []osecruritygrouprules.CreateOpts{
				{
					// Allows ipv4 traffic within this group
					Direction:     osecruritygrouprules.DirIngress,
					EtherType:     osecruritygrouprules.EtherType4,
					SecGroupID:    sg.ID,
					RemoteGroupID: sg.ID,
				},
				{
					// Allows ipv6 traffic within this group
					Direction:     osecruritygrouprules.DirIngress,
					EtherType:     osecruritygrouprules.EtherType6,
					SecGroupID:    sg.ID,
					RemoteGroupID: sg.ID,
				},
			}

			for _, opts := range rules {
				if _, err := osecruritygrouprules.Create(netClient, opts).Extract(); err != nil {
					return osErrorToTerminalError(err, "failed to create security group rule")
				}
			}
		}
	}

	return nil
}

func ensureServerGroupExists(computeClient *gophercloud.ServiceClient, n string) (string, error) {
	// Set microversion 2.15 that supports soft-anti-affinity
	old := computeClient.Microversion
	defer func() {
		computeClient.Microversion = old
	}()
	computeClient.Microversion = "2.15"

	sg, err := serverGroupByName(computeClient, n)
	if err != nil {
		if err == errNotFound {
			sg, err := osservergroups.Create(computeClient, &osservergroups.CreateOpts{
				Name:     n,
				Policies: []string{"soft-anti-affinity"},
			}).Extract()
			if err != nil {
				return "", err
			}
			return sg.ID, nil
		}
		return "", err
	}
	return sg.ID, nil
}

func getFreeFloatingIPs(client *gophercloud.ProviderClient, region string, floatingIPPool *osnetworks.Network) ([]osfloatingips.FloatingIP, error) {
	netClient, err := goopenstack.NewNetworkV2(client, gophercloud.EndpointOpts{Region: region})
	if err != nil {
		return nil, err
	}

	allPages, err := osfloatingips.List(netClient, osfloatingips.ListOpts{FloatingNetworkID: floatingIPPool.ID}).AllPages()
	if err != nil {
		return nil, err
	}

	allFIPs, err := osfloatingips.ExtractFloatingIPs(allPages)
	if err != nil {
		return nil, err
	}

	var freeFIPs []osfloatingips.FloatingIP
	for _, f := range allFIPs {
		if f.Status != openstackFloatingIPErrorStatusName && f.PortID == "" {
			freeFIPs = append(freeFIPs, f)
		}
	}

	return freeFIPs, nil
}

func createFloatingIP(client *gophercloud.ProviderClient, region, portID string, floatingIPPool *osnetworks.Network) (*osfloatingips.FloatingIP, error) {
	netClient, err := goopenstack.NewNetworkV2(client, gophercloud.EndpointOpts{Region: region})
	if err != nil {
		return nil, err
	}

	opts := osfloatingips.CreateOpts{
		FloatingNetworkID: floatingIPPool.ID,
		PortID:            portID,
	}
	return osfloatingips.Create(netClient, opts).Extract()
}

func getInstancePort(client *gophercloud.ProviderClient, region, instanceID, networkID string) (*osports.Port, error) {
	netClient, err := goopenstack.NewNetworkV2(client, gophercloud.EndpointOpts{Region: region})
	if err != nil {
		return nil, err
	}

	allPages, err := osports.List(netClient, osports.ListOpts{}).AllPages()
	if err != nil {
		return nil, err
	}

	allPorts, err := osports.ExtractPorts(allPages)
	if err != nil {
		return nil, err
	}

	for _, p := range allPorts {
		if p.NetworkID == networkID && p.DeviceID == instanceID {
			return &p, nil
		}
	}

	return nil, errNotFound
}

func getDefaultNetwork(client *gophercloud.ProviderClient, region string) (*osnetworks.Network, error) {
	networks, err := getNetworks(client, region)
	if err != nil {
		return nil, err
	}
	if len(networks) == 1 {
		return &networks[0], nil
	}

	// Networks without subnets can't be used, try finding a default by excluding them
	// However the network object itself still contains the subnet, the only difference
	// is that the subnet can not be retrieved by itself
	var candidates []osnetworks.Network
NetworkLoop:
	for _, network := range networks {
		for _, subnet := range network.Subnets {
			_, err := getSubnet(client, region, subnet)
			if err == errNotFound {
				continue
			} else if err != nil {
				return nil, err
			}
			candidates = append(candidates, network)
			continue NetworkLoop
		}
	}
	if len(candidates) == 1 {
		return &candidates[0], nil
	}

	return nil, fmt.Errorf("%d candidate networks found", len(candidates))
}

func getDefaultSubnet(client *gophercloud.ProviderClient, network *osnetworks.Network, region string) (*string, error) {
	if len(network.Subnets) == 1 {
		return &network.Subnets[0], nil
	}
	subnets, err := getSubnets(client, region, network.ID)
	if err != nil {
		return nil, err
	}
	if len(subnets) == 0 {
		return nil, errors.New("no subnets available")
	}
	return &subnets[0].ID, nil
}
