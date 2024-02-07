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

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"
)

func main() {
	// Set up flags
	flagSet := pflag.NewFlagSet("", pflag.ExitOnError)
	flagSet.Usage = func() {
		fmt.Fprintln(os.Stderr, `Usage:
	kubectl pods-on [flags] [node name or selector...]

Examples:
	kubectl pods-on node1 node2
	kubectl pods-on nodelabel=value
	kubectl pods-on "nodelabel in (value1, value2)"

Caveats:
	If this command runs slow on large clusters, tune the
	--workers and/or --strategy flags to choose a different
	query strategy.

Options:`)
		flagSet.PrintDefaults()
	}

	// Add kubectl flags
	addKlogFlags(flagSet)
	kubeConfigFlags := addConfigFlags(flagSet)
	printFlags := addPrintFlags(flagSet)
	// Add custom flags
	includeDaemonSets := flagSet.BoolP("include-daemonsets", "D", false, "Include DaemonSet Pods in the output")
	numWorkers := flagSet.Int64("workers", 20, "number of parallel workers to query pods by node")
	pprofAddr := flagSet.String("pprof-addr", "", "(dev mode) inspect the program with pprof on the given address at the end")
	strategy := flagSet.String("strategy", "", "(dev mode) choose a strategy to query pods (by-node, all-pods)")
	flagSet.Parse(os.Args[1:])

	// Start pprof server if configured
	if *pprofAddr != "" {
		klog.Infof("starting pprof server at %s", *pprofAddr)
		go func() {
			if err := http.ListenAndServe(*pprofAddr, nil); err != nil {
				klog.Warning("failed to start pprof server: ", err)
			}
		}()
	}

	posArgs := flagSet.Args()
	klog.V(3).Info("positional arguments: ", posArgs)
	selectors, nodeNames, err := parsePosArgs(posArgs)
	if err != nil {
		klog.Fatalf("failed to parse arguments: %v", err)
	}

	restCfg, err := kubeConfigFlags.ToRESTConfig()
	if err != nil {
		klog.Fatalf("failed to get REST config: %v", err)
	}
	restCfg.QPS = float32(*numWorkers) * 3
	restCfg.Burst = int(restCfg.QPS) * 3

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		klog.Fatalf("failed to create clientset: %v", err)
	}

	var heuristicTotalNodes int
	matchedNodes := sets.New[string](nodeNames...)
	if len(selectors) > 0 {
		klog.V(3).Info("resolving node selectors: ", selectors)
		out, n, err := resolveNodeNames(clientset.CoreV1().Nodes(), selectors)
		if err != nil {
			klog.Fatalf("failed to resolve nodes by selectors: %v", err)
		}
		matchedNodes = matchedNodes.Union(out)
		heuristicTotalNodes = n
	}
	klog.V(3).Infof("total nodes to query: %d", matchedNodes.Len())

	queryStrategy := podQueryStrategy(*strategy)
	if queryStrategy == "" {
		queryStrategy = chooseStrategy(heuristicTotalNodes, matchedNodes.Len())
		klog.V(1).Infof("based on nodes matched to selectors (%d/%d), using query strategy: %q",
			matchedNodes.Len(), heuristicTotalNodes, queryStrategy)
	}
	klog.V(1).Infof("pod query strategy: %q", queryStrategy)

	var pods []corev1.Pod
	switch queryStrategy {
	case queryAllPods:
		pods, err = findPodsByQueryingAllPods(context.Background(), clientset.CoreV1().Pods(""), matchedNodes)
	case queryPodPerNodeInParallel:
		klog.V(1).Infof("querying list of pods on each node in parallel (workers: %d)", *numWorkers)
		pods, err = findPodsByQueryingNodesInParallel(context.Background(), clientset.CoreV1().Pods(""), matchedNodes.UnsortedList(), *numWorkers)
	default:
		klog.Fatalf("unknown pod query strategy: %q", queryStrategy)
	}
	if err != nil {
		klog.Fatalf("failed to query pods from Kubernetes API: %v", err)
	}
	klog.V(1).Infof("query matched %d pods", len(pods))

	// Filter out daemonset pods if not requested
	if !*includeDaemonSets {
		pods = filterDaemonSetPods(pods)
	}

	// Consistent ordering for the output
	slices.SortFunc(pods, cmpPod)

	// Print the results
	if err := print(pods, printFlags); err != nil {
		klog.Fatalf("print error: %v", err)
	}

	// if pprof server is configured, keep the program running
	if *pprofAddr != "" {
		klog.Info("keeping program alive for pprof inspection")
		select {}
	}
}

// resolveNodeNames returns the names of nodes that match the given selectors,
// and the total number of nodes in the cluster.
func resolveNodeNames(nodeClient typedcorev1.NodeInterface, selectors []labels.Selector) (sets.Set[string], int, error) {
	start := time.Now()
	nodeList, err := nodeClient.List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list nodes in the cluster: %w", err)
	}
	klog.V(3).Infof("list nodes took %v (%d nodes)", time.Since(start), len(nodeList.Items))

	start = time.Now()
	nodes := sets.New[string]()
	for _, node := range nodeList.Items {
		for _, selector := range selectors {
			if selector.Matches(labels.Set(node.Labels)) {
				nodes.Insert(node.Name)
				break
			}
		}
	}
	klog.V(3).Infof("matching node selectors took %d", time.Since(start))
	return nodes, len(nodeList.Items), nil
}

// filterDaemonSetPods returns a new slice of pods that are not part of a DaemonSet.
func filterDaemonSetPods(pods []corev1.Pod) []corev1.Pod {
	var out []corev1.Pod
	for _, pod := range pods {
		var dsOwned bool
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "DaemonSet" {
				dsOwned = true
				break
			}
		}
		if !dsOwned {
			out = append(out, pod)
		}
	}
	klog.V(2).Infof("filtered out %d DaemonSet pods out of %d", len(pods)-len(out), len(pods))
	return out
}

// cmpPod sorts pods by node name, then by namespace, then by name.
func cmpPod(a, b corev1.Pod) int {
	if a.Spec.NodeName != b.Spec.NodeName {
		return strings.Compare(a.Spec.NodeName, b.Spec.NodeName)
	}
	if a.Namespace != b.Namespace {
		return strings.Compare(a.Namespace, b.Namespace)
	}
	return strings.Compare(a.Name, b.Name)
}
