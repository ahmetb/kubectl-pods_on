// Copyright 2024 Ahmet Alp Balkan
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import "k8s.io/klog/v2"

type podQueryStrategy string

const (
	queryPodPerNodeInParallel podQueryStrategy = "by-node"
	queryAllPods                               = "all-pods"
)

func chooseStrategy(heuristicTotalNodes, matchedNodes int) podQueryStrategy {
	// There's no perfect formula to determine the best strategy, as it depends on:
	//
	// * The number of pods in the cluster (–which we don't know until we query all pods)
	// * numWorkers (+ QPS/Burst) and the cluster's API server's priority/fairness settings
	//
	// Here are some examples:
	//
	// * Cluster A (200 nodes, 4000 pods)
	//   * 100 nodes matched to the selector:
	//     - "get all pods": 5s.
	//     - "get pods by node in parallel" workers=20 (matched 2000 out of 4000 pods): 10s
	//
	//   * 16 nodes matched to the selector:
	//     - "get all pods" (matched 200 pods out of 4000): 3s.
	//     - "get pods by node in parallel" workers=20: 1.5s
	//
	// * Cluster B (1000 nodes, 16,000 pods)
	//   * 850 nodes matched to the selector:
	//     - "get all pods": 20s.
	//     - "get pods by node in parallel" workers=20 (matched 11585 out of 16000 pods): 87s.
	//   * 57 nodes matched to the selector:
	//     - "get all pods" (matched 771 pods out of 16000): 22s.
	//     - "get pods by node in parallel" workers=20: 9s.

	if heuristicTotalNodes == 0 {
		// we didn't query nodes by selectors (so we don't know the total number of nodes)
		// which means user probably specified "a few nodes"
		return queryPodPerNodeInParallel
	}

	// If the number of matched nodes is less than N% of the cluster, query pods by node in parallel.
	// Otherwise, query all pods in the cluster.
	var magicRatio = 0.25

	ratio := float64(matchedNodes) / float64(heuristicTotalNodes)
	if ratio < magicRatio {
		return queryPodPerNodeInParallel
	} else {
		klog.Infof("query matched %d nodes, querying all pods in the cluster (it may be slow)", matchedNodes)
		return queryAllPods
	}
}
