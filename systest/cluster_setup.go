package main

import (
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

type DgraphCluster struct {
	TokenizerPluginsArg string

	dgraphPort string
	zeroPort   string

	dgraphPortOffset int
	zeroPortOffset   int

	binPath string
	dir     string

	zero   *exec.Cmd
	dgraph *exec.Cmd

	client *client.Dgraph
}

type DgraphClusterV struct {
	version string
	*DgraphCluster
}

func NewDgraphCluster(dir string) *DgraphCluster {
	do := freePort(x.PortGrpc)
	zo := freePort(x.PortZeroGrpc)
	return &DgraphCluster{
		dgraphPort:       strconv.Itoa(do + x.PortGrpc),
		zeroPort:         strconv.Itoa(zo + x.PortZeroGrpc),
		dgraphPortOffset: do,
		zeroPortOffset:   zo,
		dir:              dir,
		binPath:		  "$GOPATH/bin/",
	}
}

func NewDgraphClusterV(versionTag string, verPath string, dir string) (*DgraphClusterV, error) {
	do := freePort(x.PortGrpc)
	zo := freePort(7080)

	cluster := &DgraphClusterV{versionTag,
		&DgraphCluster{
			dgraphPort:       strconv.Itoa(do + x.PortGrpc),
			zeroPort:         strconv.Itoa(zo + 7080),
			dgraphPortOffset: do,
			zeroPortOffset:   zo,
			dir:              dir,
			binPath:		  verPath + "/bin/",
		},
	}

	if err := os.Setenv("GOPATH", verPath); err != nil {
		return nil, err
	}

	cmd := exec.Command("go", "get", "-d",  "-u",  "-v", "-t", "github.com/dgraph-io/dgraph/...")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "GOPATH="+verPath)
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	cmd = exec.Command("git", "checkout", "tags/" + versionTag + " -b" + versionTag)
	cmd.Dir = verPath + "/src/github.com/dgraph-io/dgraph"
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "GOPATH="+verPath)
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	cmd = exec.Command("go", "install" , "github.com/dgraph-io/dgraph/dgraph")
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "GOPATH="+verPath)
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return cluster, nil
}

func (d *DgraphCluster) StartZeroOnly() error {
	d.zero = exec.Command(d.binPath+"/dgraph",
		"zero",
		"-w=wz",
		"-o", strconv.Itoa(d.zeroPortOffset),
		"--replicas", "3",
	)
	d.zero.Dir = d.dir
	d.zero.Stdout = os.Stdout
	d.zero.Stderr = os.Stderr

	if err := d.zero.Start(); err != nil {
		return err
	}

	// Wait for dgraphzero to start listening and become the leader.
	time.Sleep(time.Second * 4)
	return nil
}

func (d *DgraphCluster) Start() error {
	if err := d.StartZeroOnly(); err != nil {
		return err
	}

	d.dgraph = exec.Command(os.ExpandEnv(d.binPath+"dgraph"),
		"server",
		"--memory_mb=4096",
		"--zero", ":"+d.zeroPort,
		"--port_offset", strconv.Itoa(d.dgraphPortOffset),
		"--custom_tokenizers", d.TokenizerPluginsArg,
	)
	d.dgraph.Dir = d.dir
	d.dgraph.Stdout = os.Stdout
	d.dgraph.Stderr = os.Stderr
	if err := d.dgraph.Start(); err != nil {
		return err
	}

	dgConn, err := grpc.Dial(":"+d.dgraphPort, grpc.WithInsecure())
	if err != nil {
		return err
	}

	// Wait for dgraph to start accepting requests. TODO: Could do this
	// programmatically by hitting the query port. This would be quicker than
	// just waiting 4 seconds (which seems to be the smallest amount of time to
	// reliably wait).
	time.Sleep(time.Second * 4)

	d.client = client.NewDgraphClient(api.NewDgraphClient(dgConn))

	return nil
}

type Node struct {
	process *exec.Cmd
	offset  string
}

func (d *DgraphCluster) AddNode(dir string) (Node, error) {
	o := strconv.Itoa(freePort(x.PortInternal))
	dgraph := exec.Command(os.ExpandEnv(d.binPath+"dgraph"),
		"server",
		"--memory_mb=4096",
		"--zero", ":"+d.zeroPort,
		"--port_offset", o,
	)
	dgraph.Dir = dir
	dgraph.Stdout = os.Stdout
	dgraph.Stderr = os.Stderr
	x.Check(os.MkdirAll(dir, os.ModePerm))
	err := dgraph.Start()

	return Node{
		process: dgraph,
		offset:  o,
	}, err
}

func (d *DgraphCluster) Close() {
	// Ignore errors
	if d.zero != nil && d.zero.Process != nil {
		d.zero.Process.Kill()
	}
	if d.dgraph != nil && d.dgraph.Process != nil {
		d.dgraph.Process.Kill()
	}
}
