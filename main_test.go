package main

import (
	"slices"
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

func TestCmpPod(t *testing.T) {
	p_n1_a_a := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "a"}, Spec: corev1.PodSpec{NodeName: "node1"}}
	p_n1_b_a := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "b", Name: "a"}, Spec: corev1.PodSpec{NodeName: "node1"}}
	p_n1_a_b := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "b"}, Spec: corev1.PodSpec{NodeName: "node1"}}
	p_n2_a_a := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "a"}, Spec: corev1.PodSpec{NodeName: "node2"}}

	v := []corev1.Pod{
		p_n2_a_a,
		p_n1_a_b,
		p_n1_b_a,
		p_n1_a_a}
	slices.SortFunc(v, cmpPod)

	require.Equal(t, []corev1.Pod{p_n1_a_a, p_n1_a_b, p_n1_b_a, p_n2_a_a}, v)
}
