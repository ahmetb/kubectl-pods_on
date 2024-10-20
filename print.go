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
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/klog/v2"
	kubectlget "k8s.io/kubectl/pkg/cmd/get"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/utils/ptr"
)

func print(resp metav1.Table, printFlags *kubectlget.PrintFlags) error {
	resourcePrinter, err := printFlags.ToPrinter()
	if err != nil {
		klog.Fatalf("failed to get printer: %v", err)
	}
	var obj runtime.Object

	switch ptr.Deref(printFlags.OutputFormat, "") {
	case "", "wide":
		// do nothing since the default format is table.
		obj = ptr.To(enhanceTable(resp))
	case "name":
		klog.Fatal("output format 'name' is not supported in this plugin since the format doesn't contain namespace references")
	default:
		// other formats (json, yaml, etc), convert to PodList
		obj = toPodList(resp)
	}
	p := printers.NewTypeSetter(scheme.Scheme).ToPrinter(resourcePrinter)

	return p.PrintObj(obj, os.Stdout)
}

func toPodList(resp metav1.Table) *corev1.PodList {
	var list corev1.PodList
	for _, row := range resp.Rows {
		list.Items = append(list.Items, *row.Object.Object.(*corev1.Pod))
	}
	list.ListMeta = resp.ListMeta
	return &list
}
