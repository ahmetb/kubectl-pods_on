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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/duration"
)

func makeTable(pods []corev1.Pod) *metav1.Table {
	table := metav1.Table{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Table",
			APIVersion: "meta.k8s.io/v1",
		},
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{
				Name:     "Node",
				Type:     "string",
				Format:   "",
				Priority: 0,
			},
			{
				Name:     "Namespace",
				Type:     "string",
				Format:   "name",
				Priority: 0,
			},
			{
				Name:     "Name",
				Type:     "string",
				Format:   "name",
				Priority: 0,
			},
			{
				Name:     "Phase",
				Type:     "string",
				Format:   "",
				Priority: 0,
			},
			{
				Name:     "Age",
				Type:     "string",
				Format:   "",
				Priority: 0,
			},
			{
				Name:     "IP",
				Type:     "string",
				Format:   "",
				Priority: 1,
			},
		},
	}

	for _, pod := range pods {
		podIP := ""
		if len(pod.Status.PodIPs) > 0 {
			podIP = pod.Status.PodIPs[0].IP
		}

		table.Rows = append(table.Rows, metav1.TableRow{
			Cells: []interface{}{
				pod.Spec.NodeName,
				pod.Namespace,
				pod.Name,
				pod.Status.Phase,
				duration.HumanDuration(time.Since(pod.CreationTimestamp.Time)),
				podIP,
			},
			Object: runtime.RawExtension{
				Object: &pod,
			},
		})
	}
	return &table
}
