package clade_test

import (
	"testing"

	"github.com/blang/semver/v4"
	"github.com/lesomnus/clade"
	"github.com/stretchr/testify/require"
)

func TestSemverMajorN(t *testing.T) {
	type Input struct {
		n  int
		vs []semver.Version
	}

	require := require.New(t)

	tcs := []struct {
		input    Input
		expected []semver.Version
	}{
		{
			input: Input{
				n: 1,
				vs: []semver.Version{
					{Major: 0, Minor: 1, Patch: 0},
					{Major: 2, Minor: 3, Patch: 4},
					{Major: 2, Minor: 3, Patch: 3},
					{Major: 1, Minor: 0, Patch: 1},
					{Major: 0, Minor: 2, Patch: 1},
					{Major: 1, Minor: 1, Patch: 1},
				},
			},
			expected: []semver.Version{
				{Major: 2, Minor: 3, Patch: 3},
				{Major: 2, Minor: 3, Patch: 4},
			},
		},
		{
			input: Input{
				n: 2,
				vs: []semver.Version{
					{Major: 0, Minor: 1, Patch: 0},
					{Major: 2, Minor: 3, Patch: 4},
					{Major: 2, Minor: 3, Patch: 3},
					{Major: 1, Minor: 0, Patch: 1},
					{Major: 0, Minor: 2, Patch: 1},
					{Major: 1, Minor: 1, Patch: 1},
				},
			},
			expected: []semver.Version{
				{Major: 1, Minor: 0, Patch: 1},
				{Major: 1, Minor: 1, Patch: 1},
				{Major: 2, Minor: 3, Patch: 3},
				{Major: 2, Minor: 3, Patch: 4},
			},
		},
	}
	for _, tc := range tcs {
		actual := clade.SemverMajorN(tc.input.n, tc.input.vs...)
		require.ElementsMatch(tc.expected, actual)
	}
}

func TestSemverMinorN(t *testing.T) {
	type Input struct {
		n  int
		vs []semver.Version
	}

	require := require.New(t)

	tcs := []struct {
		input    Input
		expected []semver.Version
	}{
		{
			input: Input{
				n: 2,
				vs: []semver.Version{
					{Major: 0, Minor: 1, Patch: 0},
					{Major: 2, Minor: 3, Patch: 4},
					{Major: 2, Minor: 3, Patch: 3},
					{Major: 1, Minor: 0, Patch: 1},
					{Major: 1, Minor: 0, Patch: 5},
					{Major: 1, Minor: 2, Patch: 5},
					{Major: 0, Minor: 1, Patch: 1},
					{Major: 1, Minor: 1, Patch: 1},
					{Major: 1, Minor: 1, Patch: 0},
				},
			},
			expected: []semver.Version{
				{Major: 0, Minor: 1, Patch: 0},
				{Major: 0, Minor: 1, Patch: 1},
				{Major: 1, Minor: 1, Patch: 0},
				{Major: 1, Minor: 1, Patch: 1},
				{Major: 1, Minor: 2, Patch: 5},
				{Major: 2, Minor: 3, Patch: 3},
				{Major: 2, Minor: 3, Patch: 4},
			},
		},
	}
	for _, tc := range tcs {
		actual := clade.SemverMinorN(tc.input.n, tc.input.vs...)
		require.ElementsMatch(tc.expected, actual)
	}
}
