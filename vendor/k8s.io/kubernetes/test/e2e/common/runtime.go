/*
Copyright 2016 The Kubernetes Authors.

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

package common

import (
	"fmt"
	"path"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/kubernetes/pkg/kubelet/images"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
)

var _ = framework.KubeDescribe("Container Runtime", func() {
	f := framework.NewDefaultFramework("container-runtime")

	Describe("blackbox test", func() {
		Context("when starting a container that exits", func() {

			/*
				Release : v1.13
				Testname: Container Runtime, Restart Policy, Pod Phases
				Description: If the restart policy is set to ‘Always’, Pod MUST be restarted when terminated, If restart policy is ‘OnFailure’, Pod MUST be started only if it is terminated with non-zero exit code. If the restart policy is ‘Never’, Pod MUST never be restarted. All these three test cases MUST verify the restart counts accordingly.
			*/
			framework.ConformanceIt("should run with the expected status [NodeConformance]", func() {
				restartCountVolumeName := "restart-count"
				restartCountVolumePath := "/restart-count"
				testContainer := v1.Container{
					Image: framework.BusyBoxImage,
					VolumeMounts: []v1.VolumeMount{
						{
							MountPath: restartCountVolumePath,
							Name:      restartCountVolumeName,
						},
					},
				}
				testVolumes := []v1.Volume{
					{
						Name: restartCountVolumeName,
						VolumeSource: v1.VolumeSource{
							EmptyDir: &v1.EmptyDirVolumeSource{Medium: v1.StorageMediumMemory},
						},
					},
				}
				testCases := []struct {
					Name          string
					RestartPolicy v1.RestartPolicy
					Phase         v1.PodPhase
					State         ContainerState
					RestartCount  int32
					Ready         bool
				}{
					{"terminate-cmd-rpa", v1.RestartPolicyAlways, v1.PodRunning, ContainerStateRunning, 2, true},
					{"terminate-cmd-rpof", v1.RestartPolicyOnFailure, v1.PodSucceeded, ContainerStateTerminated, 1, false},
					{"terminate-cmd-rpn", v1.RestartPolicyNever, v1.PodFailed, ContainerStateTerminated, 0, false},
				}
				for _, testCase := range testCases {

					// It failed at the 1st run, then succeeded at 2nd run, then run forever
					cmdScripts := `
f=%s
count=$(echo 'hello' >> $f ; wc -l $f | awk {'print $1'})
if [ $count -eq 1 ]; then
	exit 1
fi
if [ $count -eq 2 ]; then
	exit 0
fi
while true; do sleep 1; done
`
					tmpCmd := fmt.Sprintf(cmdScripts, path.Join(restartCountVolumePath, "restartCount"))
					testContainer.Name = testCase.Name
					testContainer.Command = []string{"sh", "-c", tmpCmd}
					terminateContainer := ConformanceContainer{
						PodClient:     f.PodClient(),
						Container:     testContainer,
						RestartPolicy: testCase.RestartPolicy,
						Volumes:       testVolumes,
					}
					terminateContainer.Create()
					defer terminateContainer.Delete()

					By(fmt.Sprintf("Container '%s': should get the expected 'RestartCount'", testContainer.Name))
					Eventually(func() (int32, error) {
						status, err := terminateContainer.GetStatus()
						return status.RestartCount, err
					}, ContainerStatusRetryTimeout, ContainerStatusPollInterval).Should(Equal(testCase.RestartCount))

					By(fmt.Sprintf("Container '%s': should get the expected 'Phase'", testContainer.Name))
					Eventually(terminateContainer.GetPhase, ContainerStatusRetryTimeout, ContainerStatusPollInterval).Should(Equal(testCase.Phase))

					By(fmt.Sprintf("Container '%s': should get the expected 'Ready' condition", testContainer.Name))
					Expect(terminateContainer.IsReady()).Should(Equal(testCase.Ready))

					status, err := terminateContainer.GetStatus()
					Expect(err).ShouldNot(HaveOccurred())

					By(fmt.Sprintf("Container '%s': should get the expected 'State'", testContainer.Name))
					Expect(GetContainerState(status.State)).To(Equal(testCase.State))

					By(fmt.Sprintf("Container '%s': should be possible to delete [NodeConformance]", testContainer.Name))
					Expect(terminateContainer.Delete()).To(Succeed())
					Eventually(terminateContainer.Present, ContainerStatusRetryTimeout, ContainerStatusPollInterval).Should(BeFalse())
				}
			})
		})

		Context("on terminated container", func() {
			rootUser := int64(0)
			nonRootUser := int64(10000)

			// Create and then terminate the container under defined PodPhase to verify if termination message matches the expected output. Lastly delete the created container.
			matchTerminationMessage := func(container v1.Container, expectedPhase v1.PodPhase, expectedMsg gomegatypes.GomegaMatcher) {
				container.Name = "termination-message-container"
				c := ConformanceContainer{
					PodClient:     f.PodClient(),
					Container:     container,
					RestartPolicy: v1.RestartPolicyNever,
				}

				By("create the container")
				c.Create()
				defer c.Delete()

				By(fmt.Sprintf("wait for the container to reach %s", expectedPhase))
				Eventually(c.GetPhase, ContainerStatusRetryTimeout, ContainerStatusPollInterval).Should(Equal(expectedPhase))

				By("get the container status")
				status, err := c.GetStatus()
				Expect(err).NotTo(HaveOccurred())

				By("the container should be terminated")
				Expect(GetContainerState(status.State)).To(Equal(ContainerStateTerminated))

				By("the termination message should be set")
				framework.Logf("Expected: %v to match Container's Termination Message: %v --", expectedMsg, status.State.Terminated.Message)
				Expect(status.State.Terminated.Message).Should(expectedMsg)

				By("delete the container")
				Expect(c.Delete()).To(Succeed())
			}

			It("should report termination message [LinuxOnly] if TerminationMessagePath is set [NodeConformance]", func() {
				// Cannot mount files in Windows Containers.
				container := v1.Container{
					Image:                  framework.BusyBoxImage,
					Command:                []string{"/bin/sh", "-c"},
					Args:                   []string{"/bin/echo -n DONE > /dev/termination-log"},
					TerminationMessagePath: "/dev/termination-log",
					SecurityContext: &v1.SecurityContext{
						RunAsUser: &rootUser,
					},
				}
				matchTerminationMessage(container, v1.PodSucceeded, Equal("DONE"))
			})

			It("should report termination message [LinuxOnly] if TerminationMessagePath is set as non-root user and at a non-default path [NodeConformance]", func() {
				// Cannot mount files in Windows Containers.
				container := v1.Container{
					Image:                  framework.BusyBoxImage,
					Command:                []string{"/bin/sh", "-c"},
					Args:                   []string{"/bin/echo -n DONE > /dev/termination-custom-log"},
					TerminationMessagePath: "/dev/termination-custom-log",
					SecurityContext: &v1.SecurityContext{
						RunAsUser: &nonRootUser,
					},
				}
				matchTerminationMessage(container, v1.PodSucceeded, Equal("DONE"))
			})

			It("should report termination message [LinuxOnly] from log output if TerminationMessagePolicy FallbackToLogsOnError is set [NodeConformance]", func() {
				// Cannot mount files in Windows Containers.
				container := v1.Container{
					Image:                    framework.BusyBoxImage,
					Command:                  []string{"/bin/sh", "-c"},
					Args:                     []string{"/bin/echo -n DONE; /bin/false"},
					TerminationMessagePath:   "/dev/termination-log",
					TerminationMessagePolicy: v1.TerminationMessageFallbackToLogsOnError,
				}
				matchTerminationMessage(container, v1.PodFailed, Equal("DONE"))
			})

			It("should report termination message [LinuxOnly] as empty when pod succeeds and TerminationMessagePolicy FallbackToLogsOnError is set [NodeConformance]", func() {
				// Cannot mount files in Windows Containers.
				container := v1.Container{
					Image:                    framework.BusyBoxImage,
					Command:                  []string{"/bin/sh", "-c"},
					Args:                     []string{"/bin/echo DONE; /bin/true"},
					TerminationMessagePath:   "/dev/termination-log",
					TerminationMessagePolicy: v1.TerminationMessageFallbackToLogsOnError,
				}
				matchTerminationMessage(container, v1.PodSucceeded, Equal(""))
			})

			It("should report termination message [LinuxOnly] from file when pod succeeds and TerminationMessagePolicy FallbackToLogsOnError is set [NodeConformance]", func() {
				// Cannot mount files in Windows Containers.
				container := v1.Container{
					Image:                    framework.BusyBoxImage,
					Command:                  []string{"/bin/sh", "-c"},
					Args:                     []string{"/bin/echo -n OK > /dev/termination-log; /bin/echo DONE; /bin/true"},
					TerminationMessagePath:   "/dev/termination-log",
					TerminationMessagePolicy: v1.TerminationMessageFallbackToLogsOnError,
				}
				matchTerminationMessage(container, v1.PodSucceeded, Equal("OK"))
			})
		})

		Context("when running a container with a new image", func() {

			// Images used for ConformanceContainer are not added into NodeImageWhiteList, because this test is
			// testing image pulling, these images don't need to be prepulled. The ImagePullPolicy
			// is v1.PullAlways, so it won't be blocked by framework image white list check.
			imagePullTest := func(image string, hasSecret bool, expectedPhase v1.PodPhase, expectedPullStatus bool, windowsImage bool) {
				command := []string{"/bin/sh", "-c", "while true; do sleep 1; done"}
				if windowsImage {
					// -t: Ping the specified host until stopped.
					command = []string{"ping", "-t", "localhost"}
				}
				container := ConformanceContainer{
					PodClient: f.PodClient(),
					Container: v1.Container{
						Name:            "image-pull-test",
						Image:           image,
						Command:         command,
						ImagePullPolicy: v1.PullAlways,
					},
					RestartPolicy: v1.RestartPolicyNever,
				}
				if hasSecret {
					// The service account only has pull permission
					auth := `
{
	"auths": {
		"https://gcr.io": {
			"auth": "X2pzb25fa2V5OnsKICAidHlwZSI6ICJzZXJ2aWNlX2FjY291bnQiLAogICJwcm9qZWN0X2lkIjogImF1dGhlbnRpY2F0ZWQtaW1hZ2UtcHVsbGluZyIsCiAgInByaXZhdGVfa2V5X2lkIjogImI5ZjJhNjY0YWE5YjIwNDg0Y2MxNTg2MDYzZmVmZGExOTIyNGFjM2IiLAogICJwcml2YXRlX2tleSI6ICItLS0tLUJFR0lOIFBSSVZBVEUgS0VZLS0tLS1cbk1JSUV2UUlCQURBTkJna3Foa2lHOXcwQkFRRUZBQVNDQktjd2dnU2pBZ0VBQW9JQkFRQzdTSG5LVEVFaVlMamZcbkpmQVBHbUozd3JCY2VJNTBKS0xxS21GWE5RL3REWGJRK2g5YVl4aldJTDhEeDBKZTc0bVovS01uV2dYRjVLWlNcbm9BNktuSU85Yi9SY1NlV2VpSXRSekkzL1lYVitPNkNjcmpKSXl4anFWam5mVzJpM3NhMzd0OUE5VEZkbGZycm5cbjR6UkpiOWl4eU1YNGJMdHFGR3ZCMDNOSWl0QTNzVlo1ODhrb1FBZmgzSmhhQmVnTWorWjRSYko0aGVpQlFUMDNcbnZVbzViRWFQZVQ5RE16bHdzZWFQV2dydDZOME9VRGNBRTl4bGNJek11MjUzUG4vSzgySFpydEx4akd2UkhNVXhcbng0ZjhwSnhmQ3h4QlN3Z1NORit3OWpkbXR2b0wwRmE3ZGducFJlODZWRDY2ejNZenJqNHlLRXRqc2hLZHl5VWRcbkl5cVhoN1JSQWdNQkFBRUNnZ0VBT3pzZHdaeENVVlFUeEFka2wvSTVTRFVidi9NazRwaWZxYjJEa2FnbmhFcG9cbjFJajJsNGlWMTByOS9uenJnY2p5VlBBd3pZWk1JeDFBZVF0RDdoUzRHWmFweXZKWUc3NkZpWFpQUm9DVlB6b3VcbmZyOGRDaWFwbDV0enJDOWx2QXNHd29DTTdJWVRjZmNWdDdjRTEyRDNRS3NGNlo3QjJ6ZmdLS251WVBmK0NFNlRcbmNNMHkwaCtYRS9kMERvSERoVy96YU1yWEhqOFRvd2V1eXRrYmJzNGYvOUZqOVBuU2dET1lQd2xhbFZUcitGUWFcbkpSd1ZqVmxYcEZBUW14M0Jyd25rWnQzQ2lXV2lGM2QrSGk5RXRVYnRWclcxYjZnK1JRT0licWFtcis4YlJuZFhcbjZWZ3FCQWtKWjhSVnlkeFVQMGQxMUdqdU9QRHhCbkhCbmM0UW9rSXJFUUtCZ1FEMUNlaWN1ZGhXdGc0K2dTeGJcbnplanh0VjFONDFtZHVjQnpvMmp5b1dHbzNQVDh3ckJPL3lRRTM0cU9WSi9pZCs4SThoWjRvSWh1K0pBMDBzNmdcblRuSXErdi9kL1RFalk4MW5rWmlDa21SUFdiWHhhWXR4UjIxS1BYckxOTlFKS2ttOHRkeVh5UHFsOE1veUdmQ1dcbjJ2aVBKS05iNkhabnY5Q3lqZEo5ZzJMRG5RS0JnUUREcVN2eURtaGViOTIzSW96NGxlZ01SK205Z2xYVWdTS2dcbkVzZlllbVJmbU5XQitDN3ZhSXlVUm1ZNU55TXhmQlZXc3dXRldLYXhjK0krYnFzZmx6elZZdFpwMThNR2pzTURcbmZlZWZBWDZCWk1zVXQ3Qmw3WjlWSjg1bnRFZHFBQ0xwWitaLzN0SVJWdWdDV1pRMWhrbmxHa0dUMDI0SkVFKytcbk55SDFnM2QzUlFLQmdRQ1J2MXdKWkkwbVBsRklva0tGTkh1YTBUcDNLb1JTU1hzTURTVk9NK2xIckcxWHJtRjZcbkMwNGNTKzQ0N0dMUkxHOFVUaEpKbTRxckh0Ti9aK2dZOTYvMm1xYjRIakpORDM3TVhKQnZFYTN5ZUxTOHEvK1JcbjJGOU1LamRRaU5LWnhQcG84VzhOSlREWTVOa1BaZGh4a2pzSHdVNGRTNjZwMVRESUU0MGd0TFpaRFFLQmdGaldcbktyblFpTnEzOS9iNm5QOFJNVGJDUUFKbmR3anhTUU5kQTVmcW1rQTlhRk9HbCtqamsxQ1BWa0tNSWxLSmdEYkpcbk9heDl2OUc2Ui9NSTFIR1hmV3QxWU56VnRocjRIdHNyQTB0U3BsbWhwZ05XRTZWejZuQURqdGZQSnMyZUdqdlhcbmpQUnArdjhjY21MK3dTZzhQTGprM3ZsN2VlNXJsWWxNQndNdUdjUHhBb0dBZWRueGJXMVJMbVZubEFpSEx1L0xcbmxtZkF3RFdtRWlJMFVnK1BMbm9Pdk81dFE1ZDRXMS94RU44bFA0cWtzcGtmZk1Rbk5oNFNZR0VlQlQzMlpxQ1RcbkpSZ2YwWGpveXZ2dXA5eFhqTWtYcnBZL3ljMXpmcVRaQzBNTzkvMVVjMWJSR2RaMmR5M2xSNU5XYXA3T1h5Zk9cblBQcE5Gb1BUWGd2M3FDcW5sTEhyR3pNPVxuLS0tLS1FTkQgUFJJVkFURSBLRVktLS0tLVxuIiwKICAiY2xpZW50X2VtYWlsIjogImltYWdlLXB1bGxpbmdAYXV0aGVudGljYXRlZC1pbWFnZS1wdWxsaW5nLmlhbS5nc2VydmljZWFjY291bnQuY29tIiwKICAiY2xpZW50X2lkIjogIjExMzc5NzkxNDUzMDA3MzI3ODcxMiIsCiAgImF1dGhfdXJpIjogImh0dHBzOi8vYWNjb3VudHMuZ29vZ2xlLmNvbS9vL29hdXRoMi9hdXRoIiwKICAidG9rZW5fdXJpIjogImh0dHBzOi8vYWNjb3VudHMuZ29vZ2xlLmNvbS9vL29hdXRoMi90b2tlbiIsCiAgImF1dGhfcHJvdmlkZXJfeDUwOV9jZXJ0X3VybCI6ICJodHRwczovL3d3dy5nb29nbGVhcGlzLmNvbS9vYXV0aDIvdjEvY2VydHMiLAogICJjbGllbnRfeDUwOV9jZXJ0X3VybCI6ICJodHRwczovL3d3dy5nb29nbGVhcGlzLmNvbS9yb2JvdC92MS9tZXRhZGF0YS94NTA5L2ltYWdlLXB1bGxpbmclNDBhdXRoZW50aWNhdGVkLWltYWdlLXB1bGxpbmcuaWFtLmdzZXJ2aWNlYWNjb3VudC5jb20iCn0=",
			"email": "image-pulling@authenticated-image-pulling.iam.gserviceaccount.com"
		}
	}
}`

					secret := &v1.Secret{
						Data: map[string][]byte{v1.DockerConfigJsonKey: []byte(auth)},
						Type: v1.SecretTypeDockerConfigJson,
					}
					secret.Name = "image-pull-secret-" + string(uuid.NewUUID())
					By("create image pull secret")
					_, err := f.ClientSet.CoreV1().Secrets(f.Namespace.Name).Create(secret)
					Expect(err).NotTo(HaveOccurred())
					defer f.ClientSet.CoreV1().Secrets(f.Namespace.Name).Delete(secret.Name, nil)
					container.ImagePullSecrets = []string{secret.Name}
				}
				// checkContainerStatus checks whether the container status matches expectation.
				checkContainerStatus := func() error {
					status, err := container.GetStatus()
					if err != nil {
						return fmt.Errorf("failed to get container status: %v", err)
					}
					// We need to check container state first. The default pod status is pending, If we check pod phase first,
					// and the expected pod phase is Pending, the container status may not even show up when we check it.
					// Check container state
					if !expectedPullStatus {
						if status.State.Running == nil {
							return fmt.Errorf("expected container state: Running, got: %q",
								GetContainerState(status.State))
						}
					}
					if expectedPullStatus {
						if status.State.Waiting == nil {
							return fmt.Errorf("expected container state: Waiting, got: %q",
								GetContainerState(status.State))
						}
						reason := status.State.Waiting.Reason
						if reason != images.ErrImagePull.Error() &&
							reason != images.ErrImagePullBackOff.Error() {
							return fmt.Errorf("unexpected waiting reason: %q", reason)
						}
					}
					// Check pod phase
					phase, err := container.GetPhase()
					if err != nil {
						return fmt.Errorf("failed to get pod phase: %v", err)
					}
					if phase != expectedPhase {
						return fmt.Errorf("expected pod phase: %q, got: %q", expectedPhase, phase)
					}
					return nil
				}

				// The image registry is not stable, which sometimes causes the test to fail. Add retry mechanism to make this less flaky.
				const flakeRetry = 3
				for i := 1; i <= flakeRetry; i++ {
					var err error
					By("create the container")
					container.Create()
					By("check the container status")
					for start := time.Now(); time.Since(start) < ContainerStatusRetryTimeout; time.Sleep(ContainerStatusPollInterval) {
						if err = checkContainerStatus(); err == nil {
							break
						}
					}
					By("delete the container")
					container.Delete()
					if err == nil {
						break
					}
					if i < flakeRetry {
						framework.Logf("No.%d attempt failed: %v, retrying...", i, err)
					} else {
						framework.Failf("All %d attempts failed: %v", flakeRetry, err)
					}
				}
			}

			It("should not be able to pull image from invalid registry [NodeConformance]", func() {
				image := "invalid.com/invalid/alpine:3.1"
				imagePullTest(image, false, v1.PodPending, true, false)
			})

			It("should not be able to pull non-existing image from gcr.io [NodeConformance]", func() {
				image := "k8s.gcr.io/invalid-image:invalid-tag"
				imagePullTest(image, false, v1.PodPending, true, false)
			})

			It("should be able to pull image from gcr.io [LinuxOnly] [NodeConformance]", func() {
				image := "gcr.io/google-containers/debian-base:0.4.1"
				imagePullTest(image, false, v1.PodRunning, false, false)
			})

			It("should be able to pull image from gcr.io [NodeConformance]", func() {
				framework.SkipUnlessNodeOSDistroIs("windows")
				image := "gcr.io/kubernetes-e2e-test-images/windows-nanoserver:v1"
				imagePullTest(image, false, v1.PodRunning, false, true)
			})

			It("should be able to pull image from docker hub [LinuxOnly] [NodeConformance]", func() {
				image := "alpine:3.7"
				imagePullTest(image, false, v1.PodRunning, false, false)
			})

			It("should be able to pull image from docker hub [NodeConformance]", func() {
				framework.SkipUnlessNodeOSDistroIs("windows")
				// TODO(claudiub): Switch to nanoserver image manifest list.
				image := "e2eteam/busybox:1.29"
				imagePullTest(image, false, v1.PodRunning, false, true)
			})

			It("should not be able to pull from private registry without secret [NodeConformance]", func() {
				image := "gcr.io/authenticated-image-pulling/alpine:3.7"
				imagePullTest(image, false, v1.PodPending, true, false)
			})

			It("should be able to pull from private registry with secret [LinuxOnly] [NodeConformance]", func() {
				image := "gcr.io/authenticated-image-pulling/alpine:3.7"
				imagePullTest(image, true, v1.PodRunning, false, false)
			})

			It("should be able to pull from private registry with secret [NodeConformance]", func() {
				framework.SkipUnlessNodeOSDistroIs("windows")
				image := "gcr.io/authenticated-image-pulling/windows-nanoserver:v1"
				imagePullTest(image, true, v1.PodRunning, false, true)
			})
		})
	})
})
