package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePosArgs(t *testing.T) {
	t.Run("no args", func(t *testing.T) {
		_, _, err := parsePosArgs([]string{})
		require.Error(t, err)
	})
	t.Run("node names only", func(t *testing.T) {
		selectors, nodeNames, err := parsePosArgs([]string{"node1", "node2"})
		require.NoError(t, err)
		require.Empty(t, selectors)
		require.ElementsMatch(t, []string{"node1", "node2"}, nodeNames)
	})
	t.Run("selectors only", func(t *testing.T) {
		selectors, nodeNames, err := parsePosArgs([]string{
			"foo=bar",
			"baz!=qux",
			"tier in (web,worker)",
		})
		require.NoError(t, err)
		require.Empty(t, nodeNames)
		require.Len(t, selectors, 3)
	})
	t.Run("selector parse error", func(t *testing.T) {
		_, _, err := parsePosArgs([]string{"x in "})
		require.Error(t, err)
	})
	t.Run("mixed node names and selectors", func(t *testing.T) {
		selectors, nodeNames, err := parsePosArgs([]string{
			"node1",
			"foo=bar",
			"node2",
			"baz!=qux",
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"node1", "node2"}, nodeNames)
		require.Len(t, selectors, 2)
	})
}
