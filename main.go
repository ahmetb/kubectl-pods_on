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
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/utils/ptr"
)

func main() {
	ctx := context.Background()

	// Set up flags
	flagSet := pflag.NewFlagSet("", pflag.ExitOnError)
	flagSet.Usage = func() {
		fmt.Fprintln(os.Stderr, `Usage:
	kubectl pods-on [flags] [node name or selector...]

Examples:
	kubectl pods-on node1.example.com node2.example.com
	kubectl pods-on node-label=foo
	kubectl pods-on "nodelabel in (value1, value2)"

Caveats:
	If this command runs slow on large clusters for you, it's probably because
	it's querying all pods in the cluster and doing a client-side filtering (as
	it might be faster than querying pods on each node in parallel).  You can
	manually tune the query strategy with --workers/--strategy flags.

Options:`)
		flagSet.PrintDefaults()
	}

	utilruntime.Must(metav1.AddMetaToScheme(scheme.Scheme))

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
		out, n, err := resolveNodeNames(ctx, clientset.CoreV1().Nodes(), selectors)
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

	podsRestClient, err := makePodsRESTClient(kubeConfigFlags.ToRESTConfig)
	if err != nil {
		klog.Fatalf("failed to create REST client: %v", err)
	}

	var resp metav1.Table
	switch queryStrategy {
	case queryAllPods:
		resp, err = findPodsByQueryingAllPods(ctx, podsRestClient, matchedNodes)
	case queryPodPerNodeInParallel:
		klog.V(1).Infof("querying list of pods on each node in parallel (workers: %d)", *numWorkers)
		resp, err = findPodsByQueryingNodesInParallel(ctx, podsRestClient, matchedNodes.UnsortedList(), *numWorkers)
	default:
		klog.Fatalf("unknown pod query strategy: %q", queryStrategy)
	}
	if err != nil {
		klog.Fatalf("failed to query pods from Kubernetes API: %v", err)
	}
	klog.V(1).Infof("query matched %d pods", len(resp.Rows))

	// Filter out daemonset pods if not requested
	if !*includeDaemonSets {
		resp = filterDaemonSetPods(resp)
	}

	// Consistent ordering for the output
	slices.SortFunc(resp.Rows, cmpPodRow)

	// Print the results
	if err := print(resp, printFlags); err != nil {
		klog.Fatalf("print error: %v", err)
	}

	// if pprof server is configured, keep the program running
	if *pprofAddr != "" {
		klog.Info("keeping program alive for pprof inspection")
		select {}
	}
}

func makePodsRESTClient(makeRestCfg restCfgFactory) (*rest.RESTClient, error) {
	restCfg, err := makeRestCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to create REST config for pods rest client: %w", err)
	}

	restCfg.APIPath = "/api"
	restCfg.GroupVersion = ptr.To(corev1.SchemeGroupVersion)
	restCfg.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	restCfg.UserAgent = "kubectl-pods_on"
	return rest.RESTClientFor(restCfg)
}

// resolveNodeNames returns the names of nodes that match the given selectors,
// and the total number of nodes in the cluster.
func resolveNodeNames(ctx context.Context, nodeClient typedcorev1.NodeInterface, selectors []labels.Selector) (sets.Set[string], int, error) {
	start := time.Now()
	nodeList, err := nodeClient.List(ctx, metav1.ListOptions{})
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
	klog.V(3).Infof("matching node selectors took %s", time.Since(start).Truncate(time.Millisecond))
	return nodes, len(nodeList.Items), nil
}

// filterDaemonSetPods returns a new slice of pods that are not part of a DaemonSet.
func filterDaemonSetPods(in metav1.Table) metav1.Table {
	var filtered []metav1.TableRow
	for _, podRow := range in.Rows {
		var dsOwned bool
		for _, owner := range podRow.Object.Object.(*corev1.Pod).OwnerReferences {
			if owner.Kind == "DaemonSet" {
				dsOwned = true
				break
			}
		}
		if !dsOwned {
			filtered = append(filtered, podRow)
		}
	}
	klog.V(2).Infof("filtered out %d DaemonSet pods out of %d", len(in.Rows)-len(filtered), len(in.Rows))
	in.Rows = filtered
	return in
}

// cmpPodRow sorts pods by node name, then by namespace, then by name.
func cmpPodRow(rowA, rowB metav1.TableRow) int {
	a := rowA.Object.Object.(*corev1.Pod)
	b := rowB.Object.Object.(*corev1.Pod)
	return cmpPod(*a, *b)
}

// cmpPod sorts pods by node name, then by namespace, then by pod name
// (as that's the order we print them in table layout).
func cmpPod(a, b corev1.Pod) int {
	if a.Spec.NodeName != b.Spec.NodeName {
		return strings.Compare(a.Spec.NodeName, b.Spec.NodeName)
	}
	if a.Namespace != b.Namespace {
		return strings.Compare(a.Namespace, b.Namespace)
	}
	return strings.Compare(a.Name, b.Name)
}

type restCfgFactory func() (*rest.Config, error)
