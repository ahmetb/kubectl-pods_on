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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// enhanceTable adds additional information to the table like NODE and NAMESPACE
// columns.
func enhanceTable(in metav1.Table) metav1.Table {
	// Define Node and Namespace columns
	in.ColumnDefinitions = append([]metav1.TableColumnDefinition{
		{Name: "Node", Type: "string", Priority: 0},
		{Name: "Namespace", Type: "string", Priority: 0},
	}, in.ColumnDefinitions...)

	// Add Node and Namespace values to each row
	for i := range in.Rows {
		pod := in.Rows[i].Object.Object.(*corev1.Pod)
		in.Rows[i].Cells = append([]interface{}{pod.Spec.NodeName, pod.Namespace}, in.Rows[i].Cells...)
	}

	return in
}
