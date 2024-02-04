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
	"sync"
	"time"

	"github.com/fatih/semgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"
)

func findPodsByQueryingAllPods(ctx context.Context, client typedcorev1.PodInterface, nodeNames sets.Set[string]) ([]corev1.Pod, error) {
	start := time.Now()
	pods, err := client.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list all pods in the cluster: %w", err)
	}
	klog.V(1).Infof("listing all pods took %v (found %d pods)", time.Since(start), len(pods.Items))

	var out []corev1.Pod
	for _, pod := range pods.Items {
		if nodeNames.Has(pod.Spec.NodeName) {
			out = append(out, pod)
		}
	}
	klog.V(2).Infof("matched %d pods on %d nodes", len(out), nodeNames.Len())
	return out, nil
}

// findPodsByQueryingNodesInParallel performs parallel queries to list pods by node.
func findPodsByQueryingNodesInParallel(ctx context.Context, client typedcorev1.PodInterface, nodeNames []string, numWorkers int64) ([]corev1.Pod, error) {
	var (
		mu  sync.Mutex
		out []corev1.Pod
	)

	g := semgroup.NewGroup(ctx, numWorkers)
	for _, n := range nodeNames {
		node := n
		g.Go(func() error {
			start := time.Now()
			l, err := client.List(ctx, metav1.ListOptions{
				FieldSelector: "spec.nodeName=" + node})
			if err != nil {
				return fmt.Errorf("failed to list pods on node %s: %w", node, err)
			}
			klog.V(5).Infof("listing pods on node %q took %v", node, time.Since(start).Truncate(time.Millisecond))
			mu.Lock()
			out = append(out, l.Items...)
			mu.Unlock()
			return nil
		})
	}
	err := g.Wait()
	return out, err
}
