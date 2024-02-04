package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFilterDaemonSetPods(t *testing.T) {
	p1 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1"},
	}

	p2 := *p1.DeepCopy()
	p2.Name = "p2"
	p2.OwnerReferences = []metav1.OwnerReference{{
		Kind: "ReplicaSet", Name: "rs1", UID: "rs1-uid",
	}}

	p3 := *p2.DeepCopy()
	p3.Name = "p3"
	p3.OwnerReferences = []metav1.OwnerReference{
		{Kind: "ReplicaSet", Name: "rs1", UID: "rs1-uid"},
		{Kind: "DaemonSet", Name: "ds1", UID: "ds1-uid"},
	}

	out := filterDaemonSetPods([]corev1.Pod{p1, p2, p3})
	require.ElementsMatch(t, []corev1.Pod{p1, p2}, out)
}
