//go:build e2e
// +build e2e

/*
Copyright 2026.

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

package e2e

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vijay-papanaboina/sharedresource-operator/test/utils"
)

var _ = Describe("SharedResource Secret Sync", Ordered, func() {
	const (
		sourceNS = "sr-secret-source"
		targetNS = "sr-secret-target"
	)

	BeforeAll(func() {
		By("installing CRDs")
		cmd := exec.Command("make", "install")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("waiting for controller to be ready")
		Eventually(func() error {
			cmd := exec.Command("kubectl", "get", "deployment", "-n", namespace,
				"k8s-operator-controller-manager", "-o", "jsonpath={.status.readyReplicas}")
			output, err := utils.Run(cmd)
			if err != nil {
				return err
			}
			if output != "1" {
				return fmt.Errorf("controller not ready yet")
			}
			return nil
		}, 120*time.Second, 2*time.Second).Should(Succeed())

		By("creating test namespaces")
		cmd = exec.Command("kubectl", "create", "ns", sourceNS)
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "create", "ns", targetNS)
		_, _ = utils.Run(cmd)
	})

	AfterAll(func() {
		By("cleaning up test namespaces")
		cmd := exec.Command("kubectl", "delete", "ns", sourceNS, "--ignore-not-found")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "ns", targetNS, "--ignore-not-found")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)
	})

	It("should sync a Secret to target namespace", func() {
		By("creating source Secret")
		cmd := exec.Command("kubectl", "create", "secret", "generic", "test-secret",
			"-n", sourceNS,
			"--from-literal=username=admin",
			"--from-literal=password=secret123")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("creating SharedResource CR")
		srYAML := fmt.Sprintf(`
apiVersion: platform.platform.dev/v1alpha1
kind: SharedResource
metadata:
  name: sync-secret-test
  namespace: %s
spec:
  source:
    kind: Secret
    name: test-secret
  targets:
    - namespace: %s
`, sourceNS, targetNS)

		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = stringReader(srYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for target Secret to be created")
		Eventually(func() error {
			cmd := exec.Command("kubectl", "get", "secret", "test-secret", "-n", targetNS)
			_, err := utils.Run(cmd)
			return err
		}, 60*time.Second, 2*time.Second).Should(Succeed())

		By("verifying target Secret data")
		cmd = exec.Command("kubectl", "get", "secret", "test-secret", "-n", targetNS,
			"-o", "jsonpath={.data.username}")
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(Equal("YWRtaW4=")) // base64 of "admin"

		By("cleaning up SharedResource CR")
		cmd = exec.Command("kubectl", "delete", "sharedresource", "sync-secret-test", "-n", sourceNS)
		_, _ = utils.Run(cmd)
	})
})
