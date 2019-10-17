/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package testsuites

import (
	"k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/storage/testpatterns"
)

// TestDriver represents an interface for a driver to be tested in TestSuite.
// Except for GetDriverInfo, all methods will be called at test runtime and thus
// can use framework.Skipf, framework.Fatal, Gomega assertions, etc.
type TestDriver interface {
	// GetDriverInfo returns DriverInfo for the TestDriver. This must be static
	// information.
	GetDriverInfo() *DriverInfo

	// SkipUnsupportedTest skips test if Testpattern is not
	// suitable to test with the TestDriver. It gets called after
	// parsing parameters of the test suite and before the
	// framework is initialized. Cheap tests that just check
	// parameters like the cloud provider can and should be
	// done in SkipUnsupportedTest to avoid setting up more
	// expensive resources like framework.Framework. Tests that
	// depend on a connection to the cluster can be done in
	// PrepareTest once the framework is ready.
	SkipUnsupportedTest(testpatterns.TestPattern)

	// PrepareTest is called at test execution time each time a new test case is about to start.
	// It sets up all necessary resources and returns the per-test configuration
	// plus a cleanup function that frees all allocated resources.
	PrepareTest(f *framework.Framework) (*PerTestConfig, func())
}

// TestVolume is the result of PreprovisionedVolumeTestDriver.CreateVolume.
// The only common functionality is to delete it. Individual driver interfaces
// have additional methods that work with volumes created by them.
type TestVolume interface {
	DeleteVolume()
}

// PreprovisionedVolumeTestDriver represents an interface for a TestDriver that has pre-provisioned volume
type PreprovisionedVolumeTestDriver interface {
	TestDriver
	// CreateVolume creates a pre-provisioned volume of the desired volume type.
	CreateVolume(config *PerTestConfig, volumeType testpatterns.TestVolType) TestVolume
}

// InlineVolumeTestDriver represents an interface for a TestDriver that supports InlineVolume
type InlineVolumeTestDriver interface {
	PreprovisionedVolumeTestDriver

	// GetVolumeSource returns a volumeSource for inline volume.
	// It will set readOnly and fsType to the volumeSource, if TestDriver supports both of them.
	// It will return nil, if the TestDriver doesn't support either of the parameters.
	GetVolumeSource(readOnly bool, fsType string, testVolume TestVolume) *v1.VolumeSource
}

// PreprovisionedPVTestDriver represents an interface for a TestDriver that supports PreprovisionedPV
type PreprovisionedPVTestDriver interface {
	PreprovisionedVolumeTestDriver
	// GetPersistentVolumeSource returns a PersistentVolumeSource with volume node affinity for pre-provisioned Persistent Volume.
	// It will set readOnly and fsType to the PersistentVolumeSource, if TestDriver supports both of them.
	// It will return nil, if the TestDriver doesn't support either of the parameters.
	GetPersistentVolumeSource(readOnly bool, fsType string, testVolume TestVolume) (*v1.PersistentVolumeSource, *v1.VolumeNodeAffinity)
}

// DynamicPVTestDriver represents an interface for a TestDriver that supports DynamicPV
type DynamicPVTestDriver interface {
	TestDriver
	// GetDynamicProvisionStorageClass returns a StorageClass dynamic provision Persistent Volume.
	// It will set fsType to the StorageClass, if TestDriver supports it.
	// It will return nil, if the TestDriver doesn't support it.
	GetDynamicProvisionStorageClass(config *PerTestConfig, fsType string) *storagev1.StorageClass

	// GetClaimSize returns the size of the volume that is to be provisioned ("5Gi", "1Mi").
	// The size must be chosen so that the resulting volume is large enough for all
	// enabled tests and within the range supported by the underlying storage.
	GetClaimSize() string
}

// SnapshottableTestDriver represents an interface for a TestDriver that supports DynamicSnapshot
type SnapshottableTestDriver interface {
	TestDriver
	// GetSnapshotClass returns a SnapshotClass to create snapshot.
	// It will return nil, if the TestDriver doesn't support it.
	GetSnapshotClass(config *PerTestConfig) *unstructured.Unstructured
}

// Capability represents a feature that a volume plugin supports
type Capability string

const (
	CapPersistence Capability = "persistence" // data is persisted across pod restarts
	CapBlock       Capability = "block"       // raw block mode
	CapFsGroup     Capability = "fsGroup"     // volume ownership via fsGroup
	CapExec        Capability = "exec"        // exec a file in the volume
	CapDataSource  Capability = "dataSource"  // support populate data from snapshot

	// multiple pods on a node can use the same volume concurrently;
	// for CSI, see:
	// - https://github.com/container-storage-interface/spec/pull/150
	// - https://github.com/container-storage-interface/spec/issues/178
	// - NodeStageVolume in the spec
	CapMultiPODs Capability = "multipods"
)

// DriverInfo represents static information about a TestDriver.
type DriverInfo struct {
	Name       string // Name of the driver, aka the provisioner name.
	FeatureTag string // FeatureTag for the driver

	MaxFileSize          int64               // Max file size to be tested for this driver
	SupportedFsType      sets.String         // Map of string for supported fs type
	SupportedMountOption sets.String         // Map of string for supported mount option
	RequiredMountOption  sets.String         // Map of string for required mount option (Optional)
	Capabilities         map[Capability]bool // Map that represents plugin capabilities
}

// PerTestConfig represents parameters that control test execution.
// One instance gets allocated for each test and is then passed
// via pointer to functions involved in the test.
type PerTestConfig struct {
	// The test driver for the test.
	Driver TestDriver

	// Some short word that gets inserted into dynamically
	// generated entities (pods, paths) as first part of the name
	// to make debugging easier. Can be the same for different
	// tests inside the test suite.
	Prefix string

	// The framework instance allocated for the current test.
	Framework *framework.Framework

	// If non-empty, then pods using a volume will be scheduled
	// onto the node with this name. Otherwise Kubernetes will
	// pick a node.
	ClientNodeName string

	// Some tests also support scheduling pods onto nodes with
	// these label/value pairs. As not all tests use this field,
	// a driver that absolutely needs the pods on a specific
	// node must use ClientNodeName.
	ClientNodeSelector map[string]string

	// Some test drivers initialize a storage server. This is
	// the configuration that then has to be used to run tests.
	// The values above are ignored for such tests.
	ServerConfig *framework.VolumeTestConfig
}

// GetUniqueDriverName returns unique driver name that can be used parallelly in tests
func (config *PerTestConfig) GetUniqueDriverName() string {
	return config.Driver.GetDriverInfo().Name + "-" + config.Framework.UniqueName
}
