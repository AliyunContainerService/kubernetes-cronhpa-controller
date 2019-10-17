/*
Copyright 2019 The Kubernetes Authors.

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

package storage

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	storage "k8s.io/api/storage/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"

	volumeutil "k8s.io/kubernetes/pkg/volume/util"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/storage/drivers"
	"k8s.io/kubernetes/test/e2e/storage/testsuites"
	"k8s.io/kubernetes/test/e2e/storage/utils"
	imageutils "k8s.io/kubernetes/test/utils/image"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type cleanupFuncs func()

const (
	csiNodeLimitUpdateTimeout  = 5 * time.Minute
	csiPodUnschedulableTimeout = 5 * time.Minute
	csiResizeWaitPeriod        = 5 * time.Minute
	// how long to wait for Resizing Condition on PVC to appear
	csiResizingConditionWait = 2 * time.Minute
)

var _ = utils.SIGDescribe("CSI mock volume", func() {
	type testParameters struct {
		disableAttach       bool
		attachLimit         int
		registerDriver      bool
		podInfo             *bool
		scName              string
		nodeSelectorKey     string
		enableResizing      bool // enable resizing for both CSI mock driver and storageClass.
		enableNodeExpansion bool // enable node expansion for CSI mock driver
		// just disable resizing on driver it overrides enableResizing flag for CSI mock driver
		disableResizingOnDriver bool
	}

	type mockDriverSetup struct {
		cs           clientset.Interface
		config       *testsuites.PerTestConfig
		testCleanups []cleanupFuncs
		pods         []*v1.Pod
		pvcs         []*v1.PersistentVolumeClaim
		sc           map[string]*storage.StorageClass
		driver       testsuites.TestDriver
		nodeLabel    map[string]string
		provisioner  string
		tp           testParameters
	}

	var m mockDriverSetup

	f := framework.NewDefaultFramework("csi-mock-volumes")

	init := func(tp testParameters) {
		m = mockDriverSetup{
			cs: f.ClientSet,
			sc: make(map[string]*storage.StorageClass),
			tp: tp,
		}
		cs := f.ClientSet
		var err error
		driverOpts := drivers.CSIMockDriverOpts{
			RegisterDriver:      tp.registerDriver,
			PodInfo:             tp.podInfo,
			AttachLimit:         tp.attachLimit,
			DisableAttach:       tp.disableAttach,
			EnableResizing:      tp.enableResizing,
			EnableNodeExpansion: tp.enableNodeExpansion,
		}

		// this just disable resizing on driver, keeping resizing on SC enabled.
		if tp.disableResizingOnDriver {
			driverOpts.EnableResizing = false
		}

		m.driver = drivers.InitMockCSIDriver(driverOpts)
		config, testCleanup := m.driver.PrepareTest(f)
		m.testCleanups = append(m.testCleanups, testCleanup)
		m.config = config
		m.provisioner = config.GetUniqueDriverName()

		if tp.nodeSelectorKey != "" {
			framework.AddOrUpdateLabelOnNode(m.cs, m.config.ClientNodeName, tp.nodeSelectorKey, f.Namespace.Name)
			m.nodeLabel = map[string]string{
				tp.nodeSelectorKey: f.Namespace.Name,
			}
		}

		if tp.registerDriver {
			err = waitForCSIDriver(cs, m.config.GetUniqueDriverName())
			framework.ExpectNoError(err, "Failed to get CSIDriver : %v", err)
			m.testCleanups = append(m.testCleanups, func() {
				destroyCSIDriver(cs, m.config.GetUniqueDriverName())
			})
		}
	}

	createPod := func() (*storage.StorageClass, *v1.PersistentVolumeClaim, *v1.Pod) {
		By("Creating pod")
		var sc *storagev1.StorageClass
		if dDriver, ok := m.driver.(testsuites.DynamicPVTestDriver); ok {
			sc = dDriver.GetDynamicProvisionStorageClass(m.config, "")
		}
		nodeName := m.config.ClientNodeName
		scTest := testsuites.StorageClassTest{
			Name:         m.driver.GetDriverInfo().Name,
			Provisioner:  sc.Provisioner,
			Parameters:   sc.Parameters,
			ClaimSize:    "1Gi",
			ExpectedSize: "1Gi",
		}
		if m.tp.scName != "" {
			scTest.StorageClassName = m.tp.scName
		}

		if m.tp.enableResizing {
			scTest.AllowVolumeExpansion = true
		}

		nodeSelection := testsuites.NodeSelection{
			// The mock driver only works when everything runs on a single node.
			Name: nodeName,
		}
		if len(m.nodeLabel) > 0 {
			nodeSelection = testsuites.NodeSelection{
				Selector: m.nodeLabel,
			}
		}
		class, claim, pod := startPausePod(f.ClientSet, scTest, nodeSelection, f.Namespace.Name)
		if class != nil {
			m.sc[class.Name] = class
		}
		if claim != nil {
			m.pvcs = append(m.pvcs, claim)
		}
		if pod != nil {
			m.pods = append(m.pods, pod)
		}
		return class, claim, pod
	}

	createPodWithPVC := func(pvc *v1.PersistentVolumeClaim) (*v1.Pod, error) {
		nodeName := m.config.ClientNodeName
		nodeSelection := testsuites.NodeSelection{
			Name: nodeName,
		}
		if len(m.nodeLabel) > 0 {
			nodeSelection = testsuites.NodeSelection{
				Selector: m.nodeLabel,
			}
		}
		pod, err := startPausePodWithClaim(m.cs, pvc, nodeSelection, f.Namespace.Name)
		if pod != nil {
			m.pods = append(m.pods, pod)
		}
		return pod, err
	}

	cleanup := func() {
		cs := f.ClientSet
		var errs []error

		for _, pod := range m.pods {
			By(fmt.Sprintf("Deleting pod %s", pod.Name))
			errs = append(errs, framework.DeletePodWithWait(f, cs, pod))
		}

		for _, claim := range m.pvcs {
			By(fmt.Sprintf("Deleting claim %s", claim.Name))
			claim, err := cs.CoreV1().PersistentVolumeClaims(claim.Namespace).Get(claim.Name, metav1.GetOptions{})
			if err == nil {
				cs.CoreV1().PersistentVolumeClaims(claim.Namespace).Delete(claim.Name, nil)
				framework.WaitForPersistentVolumeDeleted(cs, claim.Spec.VolumeName, framework.Poll, 2*time.Minute)
			}

		}

		for _, sc := range m.sc {
			By(fmt.Sprintf("Deleting storageclass %s", sc.Name))
			cs.StorageV1().StorageClasses().Delete(sc.Name, nil)
		}

		By("Cleaning up resources")
		for _, cleanupFunc := range m.testCleanups {
			cleanupFunc()
		}

		if len(m.nodeLabel) > 0 && len(m.tp.nodeSelectorKey) > 0 {
			framework.RemoveLabelOffNode(m.cs, m.config.ClientNodeName, m.tp.nodeSelectorKey)
		}

		err := utilerrors.NewAggregate(errs)
		Expect(err).NotTo(HaveOccurred(), "while cleaning up after test")
	}

	// The CSIDriverRegistry feature gate is needed for this test in Kubernetes 1.12.
	Context("CSI attach test using mock driver", func() {
		tests := []struct {
			name                   string
			disableAttach          bool
			deployClusterRegistrar bool
		}{
			{
				name:                   "should not require VolumeAttach for drivers without attachment",
				disableAttach:          true,
				deployClusterRegistrar: true,
			},
			{
				name:                   "should require VolumeAttach for drivers with attachment",
				deployClusterRegistrar: true,
			},
			{
				name:                   "should preserve attachment policy when no CSIDriver present",
				deployClusterRegistrar: false,
			},
		}
		for _, t := range tests {
			test := t
			It(t.name, func() {
				var err error
				init(testParameters{registerDriver: test.deployClusterRegistrar, disableAttach: test.disableAttach})
				defer cleanup()

				_, claim, pod := createPod()
				if pod == nil {
					return
				}
				err = framework.WaitForPodNameRunningInNamespace(m.cs, pod.Name, pod.Namespace)
				framework.ExpectNoError(err, "Failed to start pod: %v", err)

				By("Checking if VolumeAttachment was created for the pod")
				handle := getVolumeHandle(m.cs, claim)
				attachmentHash := sha256.Sum256([]byte(fmt.Sprintf("%s%s%s", handle, m.provisioner, m.config.ClientNodeName)))
				attachmentName := fmt.Sprintf("csi-%x", attachmentHash)
				_, err = m.cs.StorageV1beta1().VolumeAttachments().Get(attachmentName, metav1.GetOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						if !test.disableAttach {
							framework.ExpectNoError(err, "Expected VolumeAttachment but none was found")
						}
					} else {
						framework.ExpectNoError(err, "Failed to find VolumeAttachment")
					}
				}
				if test.disableAttach {
					Expect(err).To(HaveOccurred(), "Unexpected VolumeAttachment found")
				}
			})

		}
	})

	Context("CSI workload information using mock driver", func() {
		var (
			err          error
			podInfoTrue  = true
			podInfoFalse = false
		)
		tests := []struct {
			name                   string
			podInfoOnMount         *bool
			deployClusterRegistrar bool
			expectPodInfo          bool
		}{
			{
				name:                   "should not be passed when podInfoOnMount=nil",
				podInfoOnMount:         nil,
				deployClusterRegistrar: true,
				expectPodInfo:          false,
			},
			{
				name:                   "should be passed when podInfoOnMount=true",
				podInfoOnMount:         &podInfoTrue,
				deployClusterRegistrar: true,
				expectPodInfo:          true,
			},
			{
				name:                   "should not be passed when podInfoOnMount=false",
				podInfoOnMount:         &podInfoFalse,
				deployClusterRegistrar: true,
				expectPodInfo:          false,
			},
			{
				name:                   "should not be passed when CSIDriver does not exist",
				deployClusterRegistrar: false,
				expectPodInfo:          false,
			},
		}
		for _, t := range tests {
			test := t
			It(t.name, func() {
				init(testParameters{
					registerDriver: test.deployClusterRegistrar,
					scName:         "csi-mock-sc-" + f.UniqueName,
					podInfo:        test.podInfoOnMount})

				defer cleanup()

				_, _, pod := createPod()
				if pod == nil {
					return
				}
				err = framework.WaitForPodNameRunningInNamespace(m.cs, pod.Name, pod.Namespace)
				framework.ExpectNoError(err, "Failed to start pod: %v", err)
				By("Checking CSI driver logs")

				// The driver is deployed as a statefulset with stable pod names
				driverPodName := "csi-mockplugin-0"
				err = checkPodInfo(m.cs, f.Namespace.Name, driverPodName, "mock", pod, test.expectPodInfo)
				framework.ExpectNoError(err)
			})
		}
	})

	Context("CSI volume limit information using mock driver", func() {
		It("should report attach limit when limit is bigger than 0", func() {
			// define volume limit to be 2 for this test

			var err error
			nodeSelectorKey := fmt.Sprintf("attach-limit-csi-%s", f.Namespace.Name)
			init(testParameters{nodeSelectorKey: nodeSelectorKey, attachLimit: 2})
			defer cleanup()
			nodeName := m.config.ClientNodeName
			attachKey := v1.ResourceName(volumeutil.GetCSIAttachLimitKey(m.provisioner))

			nodeAttachLimit, err := checkNodeForLimits(nodeName, attachKey, m.cs)
			Expect(err).NotTo(HaveOccurred(), "while fetching node %v", err)

			Expect(nodeAttachLimit).To(Equal(2))

			_, _, pod1 := createPod()
			Expect(pod1).NotTo(BeNil(), "while creating first pod")

			err = framework.WaitForPodNameRunningInNamespace(m.cs, pod1.Name, pod1.Namespace)
			framework.ExpectNoError(err, "Failed to start pod1: %v", err)

			_, _, pod2 := createPod()
			Expect(pod2).NotTo(BeNil(), "while creating second pod")

			err = framework.WaitForPodNameRunningInNamespace(m.cs, pod2.Name, pod2.Namespace)
			framework.ExpectNoError(err, "Failed to start pod2: %v", err)

			_, _, pod3 := createPod()
			Expect(pod3).NotTo(BeNil(), "while creating third pod")
			err = waitForMaxVolumeCondition(pod3, m.cs)
			Expect(err).NotTo(HaveOccurred(), "while waiting for max volume condition on pod : %+v", pod3)
		})
	})

	Context("CSI Volume expansion [Feature:ExpandCSIVolumes]", func() {
		tests := []struct {
			name                    string
			nodeExpansionRequired   bool
			disableAttach           bool
			disableResizingOnDriver bool
			expectFailure           bool
		}{
			{
				name:                  "should expand volume without restarting pod if nodeExpansion=off",
				nodeExpansionRequired: false,
			},
			{
				name:                  "should expand volume by restarting pod if attach=on, nodeExpansion=on",
				nodeExpansionRequired: true,
			},
			{
				name:                  "should expand volume by restarting pod if attach=off, nodeExpansion=on",
				disableAttach:         true,
				nodeExpansionRequired: true,
			},
			{
				name:                    "should not expand volume if resizingOnDriver=off, resizingOnSC=on",
				disableResizingOnDriver: true,
				expectFailure:           true,
			},
		}
		for _, t := range tests {
			test := t
			It(t.name, func() {
				var err error
				tp := testParameters{
					enableResizing:          true,
					enableNodeExpansion:     test.nodeExpansionRequired,
					disableResizingOnDriver: test.disableResizingOnDriver,
				}
				// disabling attach requires drive registration feature
				if test.disableAttach {
					tp.disableAttach = true
					tp.registerDriver = true
				}

				init(tp)
				defer cleanup()

				ns := f.Namespace.Name
				sc, pvc, pod := createPod()
				Expect(pod).NotTo(BeNil(), "while creating pod for resizing")

				Expect(*sc.AllowVolumeExpansion).To(BeTrue(), "failed creating sc with allowed expansion")

				err = framework.WaitForPodNameRunningInNamespace(m.cs, pod.Name, pod.Namespace)
				framework.ExpectNoError(err, "Failed to start pod1: %v", err)

				By("Expanding current pvc")
				newSize := resource.MustParse("6Gi")
				pvc, err = expandPVCSize(pvc, newSize, m.cs)
				Expect(err).NotTo(HaveOccurred(), "While updating pvc for more size")
				Expect(pvc).NotTo(BeNil())

				pvcSize := pvc.Spec.Resources.Requests[v1.ResourceStorage]
				if pvcSize.Cmp(newSize) != 0 {
					framework.Failf("error updating pvc size %q", pvc.Name)
				}
				if test.expectFailure {
					err = waitForResizingCondition(pvc, m.cs, csiResizingConditionWait)
					Expect(err).To(HaveOccurred(), "unexpected resizing condition on PVC")
					return
				}

				By("Waiting for persistent volume resize to finish")
				err = waitForControllerVolumeResize(pvc, m.cs, csiResizeWaitPeriod)
				Expect(err).NotTo(HaveOccurred(), "While waiting for CSI PV resize to finish")

				checkPVCSize := func() {
					By("Waiting for PVC resize to finish")
					pvc, err = waitForFSResize(pvc, m.cs)
					Expect(err).NotTo(HaveOccurred(), "while waiting for PVC resize to finish")

					pvcConditions := pvc.Status.Conditions
					Expect(len(pvcConditions)).To(Equal(0), "pvc should not have conditions")
				}

				// if node expansion is not required PVC should be resized as well
				if !test.nodeExpansionRequired {
					checkPVCSize()
				} else {
					By("Checking for conditions on pvc")
					pvc, err = m.cs.CoreV1().PersistentVolumeClaims(ns).Get(pvc.Name, metav1.GetOptions{})
					Expect(err).NotTo(HaveOccurred(), "While fetching pvc after controller resize")

					inProgressConditions := pvc.Status.Conditions
					if len(inProgressConditions) > 0 {
						Expect(inProgressConditions[0].Type).To(Equal(v1.PersistentVolumeClaimFileSystemResizePending), "pvc must have fs resizing condition")
					}

					By("Deleting the previously created pod")
					err = framework.DeletePodWithWait(f, m.cs, pod)
					Expect(err).NotTo(HaveOccurred(), "while deleting pod for resizing")

					By("Creating a new pod with same volume")
					pod2, err := createPodWithPVC(pvc)
					Expect(pod2).NotTo(BeNil(), "while creating pod for csi resizing")
					Expect(err).NotTo(HaveOccurred(), "while recreating pod for resizing")

					checkPVCSize()
				}
			})
		}
	})
	Context("CSI online volume expansion [Feature:ExpandCSIVolumes][Feature:ExpandInUseVolumes]", func() {
		tests := []struct {
			name          string
			disableAttach bool
		}{
			{
				name: "should expand volume without restarting pod if attach=on, nodeExpansion=on",
			},
			{
				name:          "should expand volume without restarting pod if attach=off, nodeExpansion=on",
				disableAttach: true,
			},
		}
		for _, t := range tests {
			test := t
			It(test.name, func() {
				var err error
				params := testParameters{enableResizing: true, enableNodeExpansion: true}
				if test.disableAttach {
					params.disableAttach = true
					params.registerDriver = true
				}

				init(params)

				defer cleanup()

				sc, pvc, pod := createPod()
				Expect(pod).NotTo(BeNil(), "while creating pod for resizing")

				Expect(*sc.AllowVolumeExpansion).To(BeTrue(), "failed creating sc with allowed expansion")

				err = framework.WaitForPodNameRunningInNamespace(m.cs, pod.Name, pod.Namespace)
				framework.ExpectNoError(err, "Failed to start pod1: %v", err)

				By("Expanding current pvc")
				newSize := resource.MustParse("6Gi")
				pvc, err = expandPVCSize(pvc, newSize, m.cs)
				Expect(err).NotTo(HaveOccurred(), "While updating pvc for more size")
				Expect(pvc).NotTo(BeNil())

				pvcSize := pvc.Spec.Resources.Requests[v1.ResourceStorage]
				if pvcSize.Cmp(newSize) != 0 {
					framework.Failf("error updating pvc size %q", pvc.Name)
				}

				By("Waiting for persistent volume resize to finish")
				err = waitForControllerVolumeResize(pvc, m.cs, csiResizeWaitPeriod)
				Expect(err).NotTo(HaveOccurred(), "While waiting for PV resize to finish")

				By("Waiting for PVC resize to finish")
				pvc, err = waitForFSResize(pvc, m.cs)
				Expect(err).NotTo(HaveOccurred(), "while waiting for PVC to finish")

				pvcConditions := pvc.Status.Conditions
				Expect(len(pvcConditions)).To(Equal(0), "pvc should not have conditions")

			})
		}
	})

})

func waitForMaxVolumeCondition(pod *v1.Pod, cs clientset.Interface) error {
	var err error
	waitErr := wait.PollImmediate(10*time.Second, csiPodUnschedulableTimeout, func() (bool, error) {
		pod, err = cs.CoreV1().Pods(pod.Namespace).Get(pod.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		conditions := pod.Status.Conditions
		for _, condition := range conditions {
			matched, _ := regexp.MatchString("max.+volume.+count", condition.Message)
			if condition.Reason == v1.PodReasonUnschedulable && matched {
				return true, nil
			}

		}
		return false, nil
	})
	return waitErr
}

func checkNodeForLimits(nodeName string, attachKey v1.ResourceName, cs clientset.Interface) (int, error) {
	var attachLimit int64

	waitErr := wait.PollImmediate(10*time.Second, csiNodeLimitUpdateTimeout, func() (bool, error) {
		node, err := cs.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		limits := getVolumeLimit(node)
		var ok bool
		if len(limits) > 0 {
			attachLimit, ok = limits[attachKey]
			if ok {
				return true, nil
			}
		}
		return false, nil
	})
	return int(attachLimit), waitErr
}

func startPausePod(cs clientset.Interface, t testsuites.StorageClassTest, node testsuites.NodeSelection, ns string) (*storagev1.StorageClass, *v1.PersistentVolumeClaim, *v1.Pod) {
	class := newStorageClass(t, ns, "")
	var err error
	_, err = cs.StorageV1().StorageClasses().Get(class.Name, metav1.GetOptions{})
	if err != nil {
		class, err = cs.StorageV1().StorageClasses().Create(class)
		framework.ExpectNoError(err, "Failed to create class : %v", err)
	}

	claim := newClaim(t, ns, "")
	claim.Spec.StorageClassName = &class.Name
	claim, err = cs.CoreV1().PersistentVolumeClaims(ns).Create(claim)
	framework.ExpectNoError(err, "Failed to create claim: %v", err)

	pvcClaims := []*v1.PersistentVolumeClaim{claim}
	_, err = framework.WaitForPVClaimBoundPhase(cs, pvcClaims, framework.ClaimProvisionTimeout)
	Expect(err).NotTo(HaveOccurred(), "Failed waiting for PVC to be bound %v", err)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "pvc-volume-tester-",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "volume-tester",
					Image: imageutils.GetE2EImage(imageutils.Pause),
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "my-volume",
							MountPath: "/mnt/test",
						},
					},
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
			Volumes: []v1.Volume{
				{
					Name: "my-volume",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: claim.Name,
							ReadOnly:  false,
						},
					},
				},
			},
		},
	}

	if node.Name != "" {
		pod.Spec.NodeName = node.Name
	}
	if len(node.Selector) != 0 {
		pod.Spec.NodeSelector = node.Selector
	}

	pod, err = cs.CoreV1().Pods(ns).Create(pod)
	framework.ExpectNoError(err, "Failed to create pod: %v", err)
	return class, claim, pod
}

func startPausePodWithClaim(cs clientset.Interface, pvc *v1.PersistentVolumeClaim, node testsuites.NodeSelection, ns string) (*v1.Pod, error) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "pvc-volume-tester-",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "volume-tester",
					Image: imageutils.GetE2EImage(imageutils.Pause),
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "my-volume",
							MountPath: "/mnt/test",
						},
					},
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
			Volumes: []v1.Volume{
				{
					Name: "my-volume",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvc.Name,
							ReadOnly:  false,
						},
					},
				},
			},
		},
	}

	if node.Name != "" {
		pod.Spec.NodeName = node.Name
	}
	if len(node.Selector) != 0 {
		pod.Spec.NodeSelector = node.Selector
	}

	return cs.CoreV1().Pods(ns).Create(pod)
}

// checkPodInfo tests that NodePublish was called with expected volume_context
func checkPodInfo(cs clientset.Interface, namespace, driverPodName, driverContainerName string, pod *v1.Pod, expectPodInfo bool) error {
	expectedAttributes := map[string]string{
		"csi.storage.k8s.io/pod.name":            pod.Name,
		"csi.storage.k8s.io/pod.namespace":       namespace,
		"csi.storage.k8s.io/pod.uid":             string(pod.UID),
		"csi.storage.k8s.io/serviceAccount.name": "default",
	}
	// Load logs of driver pod
	log, err := framework.GetPodLogs(cs, namespace, driverPodName, driverContainerName)
	if err != nil {
		return fmt.Errorf("could not load CSI driver logs: %s", err)
	}
	framework.Logf("CSI driver logs:\n%s", log)
	// Find NodePublish in the logs
	foundAttributes := sets.NewString()
	logLines := strings.Split(log, "\n")
	for _, line := range logLines {
		if !strings.HasPrefix(line, "gRPCCall:") {
			continue
		}
		line = strings.TrimPrefix(line, "gRPCCall:")
		// Dummy structure that parses just volume_attributes out of logged CSI call
		type MockCSICall struct {
			Method  string
			Request struct {
				VolumeContext map[string]string `json:"volume_context"`
			}
		}
		var call MockCSICall
		err := json.Unmarshal([]byte(line), &call)
		if err != nil {
			framework.Logf("Could not parse CSI driver log line %q: %s", line, err)
			continue
		}
		if call.Method != "/csi.v1.Node/NodePublishVolume" {
			continue
		}
		// Check that NodePublish had expected attributes
		for k, v := range expectedAttributes {
			vv, found := call.Request.VolumeContext[k]
			if found && v == vv {
				foundAttributes.Insert(k)
				framework.Logf("Found volume attribute %s: %s", k, v)
			}
		}
		// Process just the first NodePublish, the rest of the log is useless.
		break
	}
	if expectPodInfo {
		if foundAttributes.Len() != len(expectedAttributes) {
			return fmt.Errorf("number of found volume attributes does not match, expected %d, got %d", len(expectedAttributes), foundAttributes.Len())
		}
		return nil
	} else {
		if foundAttributes.Len() != 0 {
			return fmt.Errorf("some unexpected volume attributes were found: %+v", foundAttributes.List())
		}
		return nil
	}
}

func waitForCSIDriver(cs clientset.Interface, driverName string) error {
	timeout := 4 * time.Minute

	framework.Logf("waiting up to %v for CSIDriver %q", timeout, driverName)
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(framework.Poll) {
		_, err := cs.StorageV1beta1().CSIDrivers().Get(driverName, metav1.GetOptions{})
		if !errors.IsNotFound(err) {
			return err
		}
	}
	return fmt.Errorf("gave up after waiting %v for CSIDriver %q.", timeout, driverName)
}

func destroyCSIDriver(cs clientset.Interface, driverName string) {
	driverGet, err := cs.StorageV1beta1().CSIDrivers().Get(driverName, metav1.GetOptions{})
	if err == nil {
		framework.Logf("deleting %s.%s: %s", driverGet.TypeMeta.APIVersion, driverGet.TypeMeta.Kind, driverGet.ObjectMeta.Name)
		// Uncomment the following line to get full dump of CSIDriver object
		// framework.Logf("%s", framework.PrettyPrint(driverGet))
		cs.StorageV1beta1().CSIDrivers().Delete(driverName, nil)
	}
}

func getVolumeHandle(cs clientset.Interface, claim *v1.PersistentVolumeClaim) string {
	// re-get the claim to the latest state with bound volume
	claim, err := cs.CoreV1().PersistentVolumeClaims(claim.Namespace).Get(claim.Name, metav1.GetOptions{})
	if err != nil {
		framework.ExpectNoError(err, "Cannot get PVC")
		return ""
	}
	pvName := claim.Spec.VolumeName
	pv, err := cs.CoreV1().PersistentVolumes().Get(pvName, metav1.GetOptions{})
	if err != nil {
		framework.ExpectNoError(err, "Cannot get PV")
		return ""
	}
	if pv.Spec.CSI == nil {
		Expect(pv.Spec.CSI).NotTo(BeNil())
		return ""
	}
	return pv.Spec.CSI.VolumeHandle
}
