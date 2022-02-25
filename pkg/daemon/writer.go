package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/glog"
	sriovnetworkv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	snclientset "github.com/k8snetworkplumbingwg/sriov-network-operator/pkg/client/clientset/versioned"
	"github.com/k8snetworkplumbingwg/sriov-network-operator/pkg/utils"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

const (
	CheckpointFileName = "sno-initial-node-state.json"
)

type NodeStateStatusWriter struct {
	client             snclientset.Interface
	node               string
	status             sriovnetworkv1.SriovNetworkNodeStateStatus
	OnHeartbeatFailure func()
	metaData           *utils.OSPMetaData
	networkData        *utils.OSPNetworkData
}

// NewNodeStateStatusWriter Create a new NodeStateStatusWriter
func NewNodeStateStatusWriter(c snclientset.Interface, n string, f func()) *NodeStateStatusWriter {
	return &NodeStateStatusWriter{
		client:             c,
		node:               n,
		OnHeartbeatFailure: f,
	}
}

// Run reads from the writer channel and sets the interface status. It will
// return if the stop channel is closed. Intended to be run via a goroutine.
func (writer *NodeStateStatusWriter) Run(stop <-chan struct{}, refresh <-chan Message, syncCh chan<- struct{}, destDir string, runonce bool, platformType utils.PlatformType) {
	glog.V(0).Infof("Run(): start writer")
	msg := Message{}

	var err error

	if platformType == utils.VirtualOpenStack {
		writer.metaData, writer.networkData, err = utils.ReadOpenstackDataFiles()
		if err != nil {
			glog.Errorf("Run(): failed to read OpenStack data files: %v", err)
			writer.metaData, writer.networkData, err = utils.FetchOpenstackData()
			if err != nil {
				glog.Errorf("Run(): failed to fetch OpenStack data: %v", err)
			}
		}
	}

	if runonce {
		glog.V(0).Info("Run(): once")
		if err := writer.pollNicStatus(platformType); err != nil {
			glog.Errorf("Run(): first poll failed: %v", err)
		}
		ns, _ := writer.setNodeStateStatus(msg)
		writer.writeCheckpointFile(ns, destDir)
		return
	}
	for {
		select {
		case <-stop:
			glog.V(0).Info("Run(): stop writer")
			return
		case msg = <-refresh:
			glog.V(0).Info("Run(): refresh trigger")
			if err := writer.pollNicStatus(platformType); err != nil {
				continue
			}
			writer.setNodeStateStatus(msg)
			if msg.syncStatus == "Succeeded" || msg.syncStatus == "Failed" {
				syncCh <- struct{}{}
			}
		case <-time.After(30 * time.Second):
			glog.V(2).Info("Run(): period refresh")
			if err := writer.pollNicStatus(platformType); err != nil {
				continue
			}
			writer.setNodeStateStatus(msg)
		}
	}
}

func (writer *NodeStateStatusWriter) pollNicStatus(platformType utils.PlatformType) error {
	glog.V(2).Info("pollNicStatus()")
	var iface []sriovnetworkv1.InterfaceExt
	var err error

	if platformType == utils.VirtualOpenStack {
		iface, err = utils.DiscoverSriovDevicesVirtual(platformType, writer.metaData, writer.networkData)
	} else {
		iface, err = utils.DiscoverSriovDevices()
	}
	if err != nil {
		return err
	}
	writer.status.Interfaces = iface

	return nil
}

func (w *NodeStateStatusWriter) updateNodeStateStatusRetry(f func(*sriovnetworkv1.SriovNetworkNodeState)) (*sriovnetworkv1.SriovNetworkNodeState, error) {
	var nodeState *sriovnetworkv1.SriovNetworkNodeState
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		n, getErr := w.getNodeState()
		if getErr != nil {
			return getErr
		}

		// Call the status modifier.
		f(n)

		var err error
		nodeState, err = w.client.SriovnetworkV1().SriovNetworkNodeStates(namespace).UpdateStatus(context.Background(), n, metav1.UpdateOptions{})
		if err != nil {
			glog.V(0).Infof("updateNodeStateStatusRetry(): fail to update the node status: %v", err)
		}
		return err
	})
	if err != nil {
		// may be conflict if max retries were hit
		return nil, fmt.Errorf("Unable to update node %v: %v", nodeState, err)
	}

	return nodeState, nil
}

func (w *NodeStateStatusWriter) setNodeStateStatus(msg Message) (*sriovnetworkv1.SriovNetworkNodeState, error) {
	nodeState, err := w.updateNodeStateStatusRetry(func(nodeState *sriovnetworkv1.SriovNetworkNodeState) {
		nodeState.Status.Interfaces = w.status.Interfaces
		if msg.lastSyncError != "" || msg.syncStatus == "Succeeded" {
			// clear lastSyncError when sync Succeeded
			nodeState.Status.LastSyncError = msg.lastSyncError
		}
		nodeState.Status.SyncStatus = msg.syncStatus

		glog.V(0).Infof("setNodeStateStatus(): syncStatus: %s, lastSyncError: %s", nodeState.Status.SyncStatus, nodeState.Status.LastSyncError)
	})
	if err != nil {
		return nil, err
	}
	return nodeState, nil
}

// getNodeState queries the kube apiserver to get the SriovNetworkNodeState CR
func (w *NodeStateStatusWriter) getNodeState() (*sriovnetworkv1.SriovNetworkNodeState, error) {
	var lastErr error
	var n *sriovnetworkv1.SriovNetworkNodeState
	err := wait.PollImmediate(10*time.Second, 5*time.Minute, func() (bool, error) {
		n, lastErr = w.client.SriovnetworkV1().SriovNetworkNodeStates(namespace).Get(context.Background(), w.node, metav1.GetOptions{})
		if lastErr == nil {
			return true, nil
		}
		glog.Warningf("getNodeState(): Failed to fetch node state %s (%v); close all connections and retry...", w.node, lastErr)
		// Use the Get() also as an client-go keepalive indicator for the TCP connection.
		w.OnHeartbeatFailure()
		return false, nil
	})
	if err != nil {
		if err == wait.ErrWaitTimeout {
			return nil, errors.Wrapf(lastErr, "Timed out trying to fetch node %s", w.node)
		}
		return nil, err
	}
	return n, nil
}

func (w *NodeStateStatusWriter) writeCheckpointFile(ns *sriovnetworkv1.SriovNetworkNodeState, destDir string) error {
	configdir := filepath.Join(destDir, CheckpointFileName)
	file, err := os.OpenFile(configdir, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	glog.Info("writeCheckpointFile(): try to decode the checkpoint file")
	if err = json.NewDecoder(file).Decode(&utils.InitialState); err != nil {
		glog.V(2).Infof("writeCheckpointFile(): fail to decode: %v", err)
		glog.Info("writeCheckpointFile(): write checkpoint file")
		if err = file.Truncate(0); err != nil {
			return err
		}
		if _, err = file.Seek(0, 0); err != nil {
			return err
		}
		if err = json.NewEncoder(file).Encode(*ns); err != nil {
			return err
		}
		utils.InitialState = *ns
	}
	return nil
}
