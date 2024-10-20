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
	"strings"
	"sync"
	"time"

	"github.com/fatih/semgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/scheme"
)

func findPodsByQueryingAllPods(ctx context.Context, restClient *rest.RESTClient, nodeNames sets.Set[string]) (metav1.Table, error) {
	resp, err := queryPods(ctx, restClient, podQueryOpts{})
	if err != nil {
		return metav1.Table{}, fmt.Errorf("failed to list pods: %w", err)
	}
	var filtered []metav1.TableRow
	for _, tableRow := range resp.Rows {
		if nodeNames.Has(tableRow.Object.Object.(*corev1.Pod).Spec.NodeName) {
			filtered = append(filtered, tableRow)
		}
	}
	resp.Rows = filtered

	klog.V(2).Infof("matched %d pods on %d nodes", len(filtered), nodeNames.Len())
	return resp, nil
}

// findPodsByQueryingNodesInParallel performs parallel queries to list pods by node.
func findPodsByQueryingNodesInParallel(ctx context.Context, restClient *rest.RESTClient, nodeNames []string, numWorkers int64) (metav1.Table, error) {
	var (
		out metav1.Table
		mu  sync.Mutex
	)

	g := semgroup.NewGroup(ctx, numWorkers)
	for _, n := range nodeNames {
		node := n
		g.Go(func() error {
			resp, err := queryPods(ctx, restClient, podQueryOpts{fieldSelectorNodeName: node})
			if err != nil {
				return fmt.Errorf("failed to list pods on node %q: %w", node, err)
			}

			mu.Lock()
			if out.Rows == nil {
				out = resp
			} else {
				// append to the existing table
				out.Rows = append(out.Rows, resp.Rows...)

				// pick the highest resource version
				if strings.Compare(resp.ResourceVersion, out.ResourceVersion) > 0 {
					out.ResourceVersion = resp.ResourceVersion
				}
			}
			mu.Unlock()
			return nil
		})
	}
	err := g.Wait()
	return out, err
}

// parsePods parses untyped pod object (RawExtension) in table rows into corev1.Pod.
func parsePods(t *metav1.Table) error {
	for i, row := range t.Rows {
		if row.Object.Object != nil {
			if _, ok := row.Object.Object.(*corev1.Pod); !ok {
				return fmt.Errorf("unexpected object type in row %d: %T (expected corev1.Pod)", i, row.Object.Object)
			}
		} else {
			// use serializer to parse pod from Object.Raw
			pod, _, err := scheme.Codecs.UniversalDeserializer().Decode(row.Object.Raw, nil, nil)
			if err != nil {
				return fmt.Errorf("failed to decode pod in row %d: %w", i, err)
			}
			row.Object.Object = pod
			t.Rows[i] = row
		}
	}
	return nil
}

type podQueryOpts struct {
	fieldSelectorNodeName string
}

func queryPods(ctx context.Context, restClient *rest.RESTClient, opts podQueryOpts) (metav1.Table, error) {
	start := time.Now()
	// query pods on each node in parallel
	var tableResp metav1.Table
	req := restClient.Get().
		Resource("pods").
		SetHeader("Accept", "application/json;as=Table;v=v1;g=meta.k8s.io,application/json").
		Param("includeObject", string(metav1.IncludeObject))

	if opts.fieldSelectorNodeName != "" {
		req = req.Param("fieldSelector", "spec.nodeName="+opts.fieldSelectorNodeName)
	}

	result := req.Do(ctx)
	if err := result.Error(); err != nil {
		return metav1.Table{}, fmt.Errorf("failed to list pods from kubernetes api: %w", err)
	}
	if err := result.Into(&tableResp); err != nil {
		return metav1.Table{}, fmt.Errorf("failed to unmarshal list pods response into metav1.Table: %w", err)
	}

	klog.V(1).Infof("listed pods, took %v (found %d pods)", time.Since(start).Truncate(time.Millisecond), len(tableResp.Rows))
	// parse raw ([]byte) pod objects into corev1.Pod
	if err := parsePods(&tableResp); err != nil {
		return metav1.Table{}, fmt.Errorf("failed to parse pods in the table response: %w", err)
	}
	return tableResp, nil
}
