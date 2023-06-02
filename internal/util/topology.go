/*
Copyright 2020 The Ceph-CSI Authors.

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

package util

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ceph/ceph-csi/internal/util/k8s"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	keySeparator   rune   = '/'
	labelSeparator string = ","
)

func k8sGetNodeLabels(nodeName string) (map[string]string, error) {
	client, err := k8s.NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("can not get node %q information, failed "+
			"to connect to Kubernetes: %w", nodeName, err)
	}

	node, err := client.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node %q information: %w", nodeName, err)
	}

	return node.GetLabels(), nil
}

// GetTopologyFromDomainLabels returns the CSI topology map, determined from
// the domain labels and their values from the CO system
// Expects domainLabels in arg to be in the format "[prefix/]<name>,[prefix/]<name>,...",.
func GetTopologyFromDomainLabels(domainLabels, nodeName, driverName string) (map[string]string, error) {
	if domainLabels == "" {
		return nil, nil
	}

	// size checks on domain label prefix
	topologyPrefix := strings.ToLower("topology." + driverName)
	const lenLimit = 63
	if len(topologyPrefix) > lenLimit {
		return nil, fmt.Errorf("computed topology label prefix %q for node exceeds length limits", topologyPrefix)
	}
	// driverName is validated, and we are adding a lowercase "topology." to it, so no validation for conformance

	// Convert passed in labels to a map, and check for uniqueness
	labelsToRead := strings.Split(domainLabels, labelSeparator)
	log.DefaultLog("passed in node labels for processing: %+v", labelsToRead)

	labelsIn := make(map[string]bool)
	labelCount := 0
	for _, label := range labelsToRead {
		// as we read the labels from k8s, and check for missing labels,
		// no label conformance checks here
		if _, ok := labelsIn[label]; ok {
			return nil, fmt.Errorf("duplicate label %q found in domain labels", label)
		}

		labelsIn[label] = true
		labelCount++
	}

	nodeLabels, err := k8sGetNodeLabels(nodeName)
	if err != nil {
		return nil, err
	}

	// Determine values for requested labels from node labels
	domainMap := make(map[string]string)
	found := 0
	for key, value := range nodeLabels {
		if _, ok := labelsIn[key]; !ok {
			continue
		}
		// label found split name component and store value
		nameIdx := strings.IndexRune(key, keySeparator)
		domain := key[nameIdx+1:]
		domainMap[domain] = value
		labelsIn[key] = false
		found++
	}

	// Ensure all labels are found
	if found != labelCount {
		missingLabels := []string{}
		for key, missing := range labelsIn {
			if missing {
				missingLabels = append(missingLabels, key)
			}
		}

		return nil, fmt.Errorf("missing domain labels %v on node %q", missingLabels, nodeName)
	}

	log.DefaultLog("list of domains processed: %+v", domainMap)

	topology := make(map[string]string)
	for domain, value := range domainMap {
		topology[topologyPrefix+"/"+domain] = value
		// TODO: when implementing domain takeover/giveback, enable a domain value that can remain pinned to the node
		// topology["topology."+driverName+"/"+domain+"-pinned"] = value
	}

	return topology, nil
}

type topologySegment struct {
	DomainLabel string `json:"domainLabel"`
	DomainValue string `json:"value"`
}

// TopologyConstrainedPool stores the pool name and a list of its associated topology domain values.
type TopologyConstrainedPool struct {
	PoolName       string            `json:"poolName"`
	DataPoolName   string            `json:"dataPool"`
	DomainSegments []topologySegment `json:"domainSegments"`
}

// GetTopologyFromRequest extracts TopologyConstrainedPools and passed in accessibility constraints
// from a CSI CreateVolume request.
func GetTopologyFromRequest(
	req *csi.CreateVolumeRequest,
) (*[]TopologyConstrainedPool, *csi.TopologyRequirement, error) {
	var topologyPools []TopologyConstrainedPool

	// check if parameters have pool configuration pertaining to topology
	topologyPoolsStr := req.GetParameters()["topologyConstrainedPools"]
	if topologyPoolsStr == "" {
		return nil, nil, nil
	}

	// check if there are any accessibility requirements in the request
	accessibilityRequirements := req.GetAccessibilityRequirements()
	if accessibilityRequirements == nil {
		return nil, nil, nil
	}

	// extract topology based pools configuration
	err := json.Unmarshal([]byte(strings.ReplaceAll(topologyPoolsStr, "\n", " ")), &topologyPools)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"failed to parse JSON encoded topology constrained pools parameter (%s): %w",
			topologyPoolsStr,
			err)
	}

	return &topologyPools, accessibilityRequirements, nil
}

// MatchPoolAndTopology returns the topology map, if the passed in pool matches any
// passed in accessibility constraints.
func MatchPoolAndTopology(topologyPools *[]TopologyConstrainedPool,
	accessibilityRequirements *csi.TopologyRequirement, poolName string,
) (string, string, map[string]string, error) {
	var topologyPool []TopologyConstrainedPool

	if topologyPools == nil || accessibilityRequirements == nil {
		return "", "", nil, nil
	}

	// find the pool in the list of topology based pools
	for _, value := range *topologyPools {
		if value.PoolName == poolName {
			topologyPool = append(topologyPool, value)

			break
		}
	}
	if len(topologyPool) == 0 {
		return "", "", nil, fmt.Errorf("none of the configured topology pools (%+v) matched passed in pool name (%s)",
			topologyPools, poolName)
	}

	return FindPoolAndTopology(&topologyPool, accessibilityRequirements)
}

// FindPoolAndTopology loops through passed in "topologyPools" and also related
// accessibility requirements, to determine which pool matches the requirement.
// The return variables are, image poolname, data poolname, and topology map of
// matched requirement.
func FindPoolAndTopology(topologyPools *[]TopologyConstrainedPool,
	accessibilityRequirements *csi.TopologyRequirement,
) (string, string, map[string]string, error) {
	if topologyPools == nil || accessibilityRequirements == nil {
		return "", "", nil, nil
	}

	// select pool that fits first topology constraint preferred requirements
	for _, topology := range accessibilityRequirements.GetPreferred() {
		topologyPool := matchPoolToTopology(topologyPools, topology)
		if topologyPool.PoolName != "" {
			return topologyPool.PoolName, topologyPool.DataPoolName, topology.GetSegments(), nil
		}
	}

	// If preferred mismatches, check requisite for a fit
	for _, topology := range accessibilityRequirements.GetRequisite() {
		topologyPool := matchPoolToTopology(topologyPools, topology)
		if topologyPool.PoolName != "" {
			return topologyPool.PoolName, topologyPool.DataPoolName, topology.GetSegments(), nil
		}
	}

	return "", "", nil, fmt.Errorf("none of the topology constrained pools matched requested "+
		"topology constraints : pools (%+v) requested topology (%+v)",
		*topologyPools, *accessibilityRequirements)
}

// matchPoolToTopology loops through passed in pools, and for each pool checks if all
// requested topology segments are present and match the request, returning the first pool
// that hence matches (or an empty string if none match).
func matchPoolToTopology(topologyPools *[]TopologyConstrainedPool, topology *csi.Topology) TopologyConstrainedPool {
	domainMap := extractDomainsFromlabels(topology)

	// check if any pool matches all the domain keys and values
	for _, topologyPool := range *topologyPools {
		mismatch := false
		// match all pool topology labels to requested topology
		for _, segment := range topologyPool.DomainSegments {
			if domainValue, ok := domainMap[segment.DomainLabel]; !ok || domainValue != segment.DomainValue {
				mismatch = true

				break
			}
		}

		if mismatch {
			continue
		}

		return topologyPool
	}

	return TopologyConstrainedPool{}
}

// extractDomainsFromlabels returns the domain name map, from passed in domain segments,
// which is of the form [prefix/]<name>.
func extractDomainsFromlabels(topology *csi.Topology) map[string]string {
	domainMap := make(map[string]string)
	for domainKey, value := range topology.GetSegments() {
		domainIdx := strings.IndexRune(domainKey, keySeparator)
		domain := domainKey[domainIdx+1:]
		domainMap[domain] = value
	}

	return domainMap
}
