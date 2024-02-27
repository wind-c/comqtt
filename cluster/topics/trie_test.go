// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package topics

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	index := New()
	require.NotNil(t, index)
	require.NotNil(t, index.Root)
}

func BenchmarkNew(b *testing.B) {
	for n := 0; n < b.N; n++ {
		New()
	}
}

func TestPoperate(t *testing.T) {
	index := New()
	child := index.poperate("path/to/my/mqtt")
	require.Equal(t, "mqtt", child.Key)
	require.NotNil(t, index.Root.Leaves["path"].Leaves["to"].Leaves["my"].Leaves["mqtt"])

	child = index.poperate("a/b/c/d/e")
	require.Equal(t, "e", child.Key)
	child = index.poperate("a/b/c/c/a")
	require.Equal(t, "a", child.Key)
}

func BenchmarkPoperate(b *testing.B) {
	index := New()
	for n := 0; n < b.N; n++ {
		index.poperate("path/to/my/mqtt")
	}
}

func TestUnpoperate(t *testing.T) {
	index := New()
	index.Subscribe("path/to/my/mqtt")
	require.Equal(t, index.Root.Leaves["path"].Leaves["to"].Leaves["my"].Leaves["mqtt"].Count, 1)

	index.Subscribe("path/to/another/mqtt")
	require.Equal(t, index.Root.Leaves["path"].Leaves["to"].Leaves["another"].Leaves["mqtt"].Count, 1)

	index.unpoperate("path/to/my/mqtt")
	require.Nil(t, index.Root.Leaves["path"].Leaves["to"].Leaves["my"])

	index.unpoperate("path/to/whatever") // unsubscribe client
	require.Nil(t, index.Root.Leaves["path"].Leaves["to"].Leaves["my"])
}

func BenchmarkUnpoperate(b *testing.B) {
	index := New()
	for n := 0; n < b.N; n++ {
		index.poperate("path/to/my/mqtt")
	}
}

func TestSubscribeOK(t *testing.T) {
	index := New()

	q := index.Subscribe("path/to/#")
	require.Equal(t, true, q)

	q = index.Subscribe("path/to/+/mqtt")
	require.Equal(t, true, q)

	q = index.Subscribe("path/to/my/mqtt")
	require.Equal(t, true, q)

	filters := index.Scan("path/to/my/mqtt", make([]string, 0))
	require.Equal(t, "path/to/my/mqtt", filters[0])
	require.Equal(t, "path/to/+/mqtt", filters[1])
	require.Equal(t, "path/to/#", filters[2])

	filters = index.Scan("path/to/my", make([]string, 0))
	require.Equal(t, "path/to/#", filters[0])

	filters = index.Scan("path/to", make([]string, 0))
	require.Equal(t, "path/to/#", filters[0])

	filters = index.Scan("path", make([]string, 0))
	require.Equal(t, 0, len(filters))

	q = index.Subscribe("path/to/another/mqtt")
	require.Equal(t, true, q)

	filters = index.Scan("path/to/another/mqtt", make([]string, 0))
	require.Equal(t, "path/to/another/mqtt", filters[0])
	require.Equal(t, "path/to/+/mqtt", filters[1])
	require.Equal(t, "path/to/#", filters[2])

	q = index.Subscribe("path/+")
	require.Equal(t, true, q)
	filters = index.Scan("path/+", make([]string, 0))
	require.Equal(t, "path/+", filters[0])

	filters = index.Scan("a/b/c", make([]string, 0))
	require.Equal(t, 0, len(filters))

	q = index.Subscribe("#")
	require.Equal(t, true, q)
	filters = index.Scan("#", make([]string, 0))
	require.Equal(t, "#", filters[0])

	filters = index.Scan("a/b/c", make([]string, 0))
	require.Equal(t, "#", filters[0])

	require.Equal(t, index.Root.Leaves["path"].Leaves["to"].Leaves["my"].Leaves["mqtt"].Count, 1)
	require.Equal(t, "mqtt", index.Root.Leaves["path"].Leaves["to"].Leaves["my"].Leaves["mqtt"].Key)
	require.Equal(t, index.Root.Leaves["path"], index.Root.Leaves["path"].Leaves["to"].Parent)

	require.Equal(t, index.Root.Leaves["path"].Leaves["to"].Leaves["+"].Leaves["mqtt"].Count, 1)

	require.Equal(t, index.Root.Leaves["path"].Leaves["to"].Leaves["another"].Leaves["mqtt"].Count, 1)
	require.Equal(t, index.Root.Leaves["path"].Leaves["+"].Count, 1)
	require.Equal(t, index.Root.Leaves["#"].Count, 1)
}

func BenchmarkSubscribe(b *testing.B) {
	index := New()
	for n := 0; n < b.N; n++ {
		index.Subscribe("path/to/mqtt/basic")
	}
}

func TestUnsubscribe(t *testing.T) {
	index := New()
	index.Subscribe("path/to/my/mqtt")
	index.Subscribe("path/to/+/mqtt")
	index.Subscribe("path/to/stuff")
	index.Subscribe("path/to/stuff")
	index.Subscribe("#")
	require.Equal(t, index.Root.Leaves["path"].Leaves["to"].Leaves["my"].Leaves["mqtt"].Count, 1)
	require.Equal(t, index.Root.Leaves["path"].Leaves["to"].Leaves["+"].Leaves["mqtt"].Count, 1)
	require.Equal(t, index.Root.Leaves["path"].Leaves["to"].Leaves["stuff"].Count, 2)
	require.Equal(t, index.Root.Leaves["#"].Count, 1)

	ok := index.Unsubscribe("path/to/my/mqtt")
	require.Equal(t, true, ok)
	require.Nil(t, index.Root.Leaves["path"].Leaves["to"].Leaves["my"])
	require.Equal(t, index.Root.Leaves["path"].Leaves["to"].Count, 0)

	ok = index.Unsubscribe("path/to/stuff")
	require.Equal(t, true, ok)
	require.NotNil(t, index.Root.Leaves["path"].Leaves["to"].Leaves["stuff"])
	require.Equal(t, index.Root.Leaves["path"].Leaves["to"].Leaves["stuff"].Count, 1)

	ok = index.Unsubscribe("#")
	require.Nil(t, index.Root.Leaves["#"])

	require.Equal(t, index.Root.Leaves["path"].Leaves["to"].Leaves["stuff"].Count, 1)

	ok = index.Unsubscribe("fdasfdas/dfsfads/sa")
	require.Equal(t, false, ok)

}

// This benchmark is Unsubscribe-Subscribe
func BenchmarkUnsubscribe(b *testing.B) {
	index := New()

	for n := 0; n < b.N; n++ {
		index.Subscribe("path/to/my/mqtt")
		index.Unsubscribe("path/to/mqtt/basic")
	}
}

func TestSubscribersFind(t *testing.T) {
	tt := []struct {
		filter string
		topic  string
		len    int
	}{
		{
			filter: "a",
			topic:  "a",
			len:    1,
		},
		{
			filter: "a/",
			topic:  "a",
			len:    0,
		},
		{
			filter: "a/",
			topic:  "a/",
			len:    1,
		},
		{
			filter: "/a",
			topic:  "/a",
			len:    1,
		},
		{
			filter: "path/to/my/mqtt",
			topic:  "path/to/my/mqtt",
			len:    1,
		},
		{
			filter: "path/to/+/mqtt",
			topic:  "path/to/my/mqtt",
			len:    1,
		},
		{
			filter: "+/to/+/mqtt",
			topic:  "path/to/my/mqtt",
			len:    1,
		},
		{
			filter: "#",
			topic:  "path/to/my/mqtt",
			len:    1,
		},
		{
			filter: "+/+/+/+",
			topic:  "path/to/my/mqtt",
			len:    1,
		},
		{
			filter: "+/+/+/#",
			topic:  "path/to/my/mqtt",
			len:    1,
		},
		{
			filter: "zen/#",
			topic:  "zen",
			len:    1,
		},
		{
			filter: "+/+/#",
			topic:  "path/to/my/mqtt",
			len:    1,
		},
		{
			filter: "path/to/",
			topic:  "path/to/my/mqtt",
			len:    0,
		},
		{
			filter: "#/stuff",
			topic:  "path/to/my/mqtt",
			len:    0,
		},
		{
			filter: "$SYS/#",
			topic:  "$SYS/info",
			len:    1,
		},
		{
			filter: "#",
			topic:  "$SYS/info",
			len:    0,
		},
		{
			filter: "+/info",
			topic:  "$SYS/info",
			len:    0,
		},
	}

	for i, check := range tt {
		index := New()
		index.Subscribe(check.filter)
		res := index.Scan(check.topic, make([]string, 0))
		require.Equal(t, check.len, len(res), fmt.Sprintf("case %d did not meet expectations - '%+v'", i, check))
	}

}

func TestIsolateParticle(t *testing.T) {
	particle, hasNext := isolateParticle("path/to/my/mqtt", 0)
	require.Equal(t, "path", particle)
	require.Equal(t, true, hasNext)
	particle, hasNext = isolateParticle("path/to/my/mqtt", 1)
	require.Equal(t, "to", particle)
	require.Equal(t, true, hasNext)
	particle, hasNext = isolateParticle("path/to/my/mqtt", 2)
	require.Equal(t, "my", particle)
	require.Equal(t, true, hasNext)
	particle, hasNext = isolateParticle("path/to/my/mqtt", 3)
	require.Equal(t, "mqtt", particle)
	require.Equal(t, false, hasNext)

	particle, hasNext = isolateParticle("/path/", 0)
	require.Equal(t, "", particle)
	require.Equal(t, true, hasNext)
	particle, hasNext = isolateParticle("/path/", 1)
	require.Equal(t, "path", particle)
	require.Equal(t, true, hasNext)
	particle, hasNext = isolateParticle("/path/", 2)
	require.Equal(t, "", particle)
	require.Equal(t, false, hasNext)

	particle, hasNext = isolateParticle("a/b/c/+/+", 3)
	require.Equal(t, "+", particle)
	require.Equal(t, true, hasNext)
	particle, hasNext = isolateParticle("a/b/c/+/+", 4)
	require.Equal(t, "+", particle)
	require.Equal(t, false, hasNext)
}

func BenchmarkIsolateParticle(b *testing.B) {
	for n := 0; n < b.N; n++ {
		isolateParticle("path/to/my/mqtt", 3)
	}
}

func TestConvertSharedFilter(t *testing.T) {
	srcFilter := "$share/group1/filter/1"

	group, destFilter := convertSharedFilter(srcFilter)
	require.Equal(t, "group1/", group)
	require.Equal(t, "filter/1", destFilter)
}

func TestRestoreShareFilter(t *testing.T) {
	filter := "filter"
	sharedGroups := []string{"Group1/", "Group2/", "Group3/"}

	result := restoreShareFilter(filter, sharedGroups)
	expectedResult := []string{"$share/Group1/filter", "$share/Group2/filter", "$share/Group3/filter"}

	require.EqualValues(t, expectedResult, result)
}
