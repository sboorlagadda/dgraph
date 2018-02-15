package main

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/stretchr/testify/require"
)

func TestCompatibility(t *testing.T) {
	// old cluster
	versionPath, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(versionPath)

	dir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	old_cluster, err := NewDgraphClusterV("v1.0.2", versionPath, dir)
	require.NoError(t, err)
	defer old_cluster.Close()

	cluster := NewDgraphCluster(dir)
	defer cluster.Close()

	ctx := context.Background()
	require.NoError(t, old_cluster.Start())

	check(t, old_cluster.client.Alter(ctx, &api.Operation{
		Schema: `list: [string] .`,
	}))

	txn := old_cluster.client.NewTxn()
	defer txn.Discard(ctx)
	_, err = txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			<0x1> <name> "abc" .
			<0x1> <name> "abc_en"@en .
			<0x1> <name> "abc_nl"@nl .
			<0x2> <name> "abc_hi"@hi .
			<0x2> <name> "abc_ci"@ci .
			<0x2> <name> "abc_ja"@ja .
			<0x3> <name> "abcd" .
			<0x1> <number> "99"^^<xs:int> .

			<0x1> <list> "first" .
			<0x1> <list> "first_en"@en .
			<0x1> <list> "first_it"@it .
			<0x1> <list> "second" .
		`),
	})

	resp, err := old_cluster.client.NewTxn().Query(context.Background(), `
	{
		q(func: uid(0x1,0x2,0x3)) {
			expand(_all_)
		}
	}
	`)
	check(t, err)

	CompareJSON(t, `
	{
		"q": [
			{
				"name": "abcd"
			},
			{
			    "name@ci": "abc_ci",
			    "name@hi": "abc_hi",
			    "name@ja": "abc_ja"
			},
			{
				"name@en": "abc_en",
				"name@nl": "abc_nl",
				"name": "abc",
				"number": 99,
				"list": [
					"second",
					"first"
				],
				"list@en": "first_en",
				"list@it": "first_it"
			}
		]
	}
	`, string(resp.GetJson()))

	//close old cluster
	old_cluster.Close()

	//start a new cluster
	require.NoError(t, cluster.Start())

	resp, err = old_cluster.client.NewTxn().Query(context.Background(), `
	{
		q(func: uid(0x1,0x2,0x3)) {
			expand(_all_)
		}
	}
	`)
	check(t, err)

	CompareJSON(t, `
	{
		"q": [
			{
				"name": "abcd"
			},
			{
			    "name@ci": "abc_ci",
			    "name@hi": "abc_hi",
			    "name@ja": "abc_ja"
			},
			{
				"name@en": "abc_en",
				"name@nl": "abc_nl",
				"name": "abc",
				"number": 99,
				"list": [
					"second",
					"first"
				],
				"list@en": "first_en",
				"list@it": "first_it"
			}
		]
	}
	`, string(resp.GetJson()))
}
