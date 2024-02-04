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
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	kubectlget "k8s.io/kubectl/pkg/cmd/get"
)

func addKlogFlags(flagSet *pflag.FlagSet) {
	klogFlagSet := flag.NewFlagSet("ignored", flag.ExitOnError)
	klog.InitFlags(klogFlagSet)
	flagSet.AddGoFlagSet(klogFlagSet)
}

func addConfigFlags(flagSet *pflag.FlagSet) *genericclioptions.ConfigFlags {
	kubeCfgFlags := genericclioptions.NewConfigFlags(false)
	kubeCfgFlags.AddFlags(flagSet)
	return kubeCfgFlags
}

func addPrintFlags(flagSet *pflag.FlagSet) *kubectlget.PrintFlags {
	dummyCobraCmd := &cobra.Command{}
	printFlags := kubectlget.NewGetPrintFlags()
	printFlags.AddFlags(dummyCobraCmd)
	flagSet.AddFlagSet(dummyCobraCmd.Flags())
	return printFlags
}

func parsePosArgs(posArgs []string) (selectors []labels.Selector, nodeNames []string, err error) {
	if len(posArgs) == 0 {
		return nil, nil, errors.New("no positional arguments specified. specify node names or node selectors")
	}
	for _, arg := range posArgs {
		// selector heuristic: contains = or " "
		if !strings.ContainsAny(arg, "= ") {
			nodeNames = append(nodeNames, arg)
			continue
		}
		selector, err := labels.Parse(arg)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse node selector %q: %w", arg, err)
		}
		selectors = append(selectors, selector)
	}
	return
}
