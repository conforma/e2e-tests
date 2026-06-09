package contract

import (
	"fmt"
	"time"

	ecp "github.com/conforma/crds/api/v1alpha1"
	"github.com/conforma/e2e-tests/pkg/clients/common"
	"github.com/conforma/e2e-tests/pkg/constants"
	"github.com/conforma/e2e-tests/pkg/framework"
	"github.com/conforma/e2e-tests/pkg/utils/contract"
	"github.com/conforma/e2e-tests/pkg/utils/tekton"
	"github.com/devfile/library/v2/pkg/util"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

var _ = framework.ConformaSuiteDescribe("Conforma E2E tests", ginkgo.Label("ec"), func() {

	defer ginkgo.GinkgoRecover()

	var namespace string
	var fwk *framework.Framework
	var tektonChainsNs string

	ginkgo.AfterEach(framework.ReportFailure(&fwk))

	ginkgo.BeforeAll(func() {
		var err error
		fwk, err = framework.NewFramework(framework.GetGeneratedNamespace(constants.TEKTON_CHAINS_E2E_USER))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(fwk.UserNamespace).NotTo(gomega.BeEmpty(), "failed to create sandbox user")
		namespace = fwk.UserNamespace

		tektonChainsNs, err = fwk.AsKubeAdmin.TektonController.GetTektonChainsNamespace()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.Context("infrastructure is running", ginkgo.Label("pipeline"), func() {
		ginkgo.It("verifies if the chains controller is running", func() {
			err := fwk.AsKubeAdmin.CommonController.WaitForPodSelector(fwk.AsKubeAdmin.CommonController.IsPodRunning, tektonChainsNs, "app", "tekton-chains-controller", 60, 100)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("verifies the signing secret is present", func() {
			timeout := time.Minute * 5
			interval := time.Second * 1

			gomega.Eventually(func() bool {
				config, err := fwk.AsKubeAdmin.CommonController.GetSecret(tektonChainsNs, constants.TEKTON_CHAINS_SIGNING_SECRETS_NAME)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				_, private := config.Data["cosign.key"]
				_, public := config.Data["cosign.pub"]
				_, password := config.Data["cosign.password"]

				return private && public && password
			}, timeout, interval).Should(gomega.BeTrue(), fmt.Sprintf("timed out when waiting for Tekton Chains signing secret %q to be present in %q namespace", constants.TEKTON_CHAINS_SIGNING_SECRETS_NAME, tektonChainsNs))
		})
	})

	ginkgo.Context("test creating and signing an image and task", ginkgo.Label("pipeline"), func() {
		var buildPipelineRunName, image, imageWithDigest string
		var pipelineRunTimeout int
		var defaultECP *ecp.EnterpriseContractPolicy

		ginkgo.BeforeAll(func() {
			buildPipelineRunName = fmt.Sprintf("buildah-demo-%s", util.GenerateRandomString(10))
			image = fmt.Sprintf("quay.io/%s/test-images:%s", framework.GetQuayIOOrganization(), buildPipelineRunName)

			gomega.Expect(fwk.AsKubeAdmin.CommonController.CreateQuayRegistrySecret(namespace)).To(gomega.Succeed())

			pipelineRunTimeout = int(time.Duration(20) * time.Minute)

			// Red Hat Konflux and upstream/operator Konflux have different defaults
			// for policy and data sources. Replace default operator Konflux ECP with
			// Red Hat Konflux since e2e testing focusing on Red Hat Konflux stability.
			defaultECP = &ecp.EnterpriseContractPolicy{}
			defaultECP.Spec = ecp.EnterpriseContractPolicySpec{
				Name:        "Red Hat",
				Description: "Includes the full set of rules and policies required internally by Red Hat when building Red Hat products.",
				Sources: []ecp.Source{
					{
						Name: "Default",
						Policy: []string{
							"oci::quay.io/enterprise-contract/ec-release-policy:konflux",
						},
						Data: []string{
							"git::github.com/release-engineering/rhtap-ec-policy//data?ref=main",
							"oci::quay.io/konflux-ci/tekton-catalog/data-acceptable-bundles:latest",
							"oci::quay.io/konflux-ci/konflux-vanguard/data-acceptable-bundles:latest",
							"oci::quay.io/konflux-ci/integration-service-catalog/data-acceptable-bundles:latest",
						},
						Config: &ecp.SourceConfig{
							Include: []string{"@redhat"},
						},
					},
				},
			}
			var err error

			bundles, err := fwk.AsKubeAdmin.TektonController.NewBundles()
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			dockerBuildBundle := bundles.DockerBuildOCITAMinBundle
			dockerBuildPipelineName := "docker-build-oci-ta-min"
			if dockerBuildBundle == "" {
				dockerBuildBundle = bundles.DockerBuildBundle
				dockerBuildPipelineName = "docker-build"
			}
			gomega.Expect(dockerBuildBundle).NotTo(gomega.Equal(""), "Can't continue without a docker-build pipeline got from selector config")

			pvcName := "app-studio-default-workspace"
			gomega.Expect(tekton.CreatePVC(
				fwk.AsKubeAdmin.CommonController.KubeInterface().CoreV1().PersistentVolumeClaims(namespace),
				pvcName,
			)).Should(gomega.Or(gomega.Succeed(), gomega.MatchError(gomega.ContainSubstring("already exists"))))

			pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(tekton.BuildahDemo{Image: image, Bundle: dockerBuildBundle, PipelineName: dockerBuildPipelineName, Namespace: namespace, Name: buildPipelineRunName}, namespace, pipelineRunTimeout)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pr.Name).To(gomega.Equal(buildPipelineRunName))
			gomega.Expect(pr.Namespace).To(gomega.Equal(namespace))
			gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())
			ginkgo.GinkgoWriter.Printf("The pipeline named %q in namespace %q succeeded\n", pr.Name, pr.Namespace)

			pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.GinkgoWriter.Printf("PipelineRun %s/%s childReferences:\n", pr.Namespace, pr.Name)
			for _, chr := range pr.Status.ChildReferences {
				ginkgo.GinkgoWriter.Printf("  task=%s name=%s\n", chr.PipelineTaskName, chr.Name)
			}

			digest, err := fwk.AsKubeAdmin.TektonController.GetTaskRunResult(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "build-container", "IMAGE_DIGEST")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			i, err := fwk.AsKubeAdmin.TektonController.GetTaskRunResult(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "build-container", "IMAGE_URL")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(i).To(gomega.Equal(image))

			imageWithDigest = fmt.Sprintf("%s@%s", image, digest)
			ginkgo.GinkgoWriter.Printf("The image signed by Tekton Chains is %s\n", imageWithDigest)
		})

		ginkgo.It("creates signature and attestation", func() {
			err := fwk.AsKubeAdmin.TektonController.AwaitAttestationAndSignature(imageWithDigest, constants.ChainsAttestationTimeout)
			gomega.Expect(err).NotTo(
				gomega.HaveOccurred(),
				"Could not find .att or .sig ImageStreamTags within the %s timeout. "+
					"Most likely the chains-controller did not create those in time. "+
					"Look at the chains-controller logs.",
				constants.ChainsAttestationTimeout.String(),
			)
			ginkgo.GinkgoWriter.Printf("Cosign verify pass with .att and .sig ImageStreamTags found for %s\n", imageWithDigest)
		})

		ginkgo.Context("verify-enterprise-contract task", func() {
			var generator tekton.VerifyEnterpriseContract
			var rekorHost string
			var verifyECTaskBundle string
			publicSecretName := "cosign-public-key"

			ginkgo.BeforeAll(func() {
				publicKey, err := fwk.AsKubeAdmin.TektonController.GetTektonChainsPublicKey()
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				ginkgo.GinkgoWriter.Printf("Copy public key from %s/signing-secrets to a new secret\n", tektonChainsNs)
				gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdateSigningSecret(
					publicKey, publicSecretName, namespace)).To(gomega.Succeed())

				rekorHost, err = fwk.AsKubeAdmin.TektonController.GetRekorHost()
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				ginkgo.GinkgoWriter.Printf("Configured Rekor host: %s\n", rekorHost)

				cm, err := fwk.AsKubeAdmin.CommonController.GetConfigMap("ec-defaults", "enterprise-contract-service")
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				verifyECTaskBundle = cm.Data["verify_ec_task_bundle"]
				gomega.Expect(verifyECTaskBundle).ToNot(gomega.BeEmpty())
				ginkgo.GinkgoWriter.Printf("Using verify EC task bundle: %s\n", verifyECTaskBundle)
			})

			ginkgo.BeforeEach(func() {
				generator = tekton.VerifyEnterpriseContract{
					TaskBundle:          verifyECTaskBundle,
					Name:                "verify-enterprise-contract",
					Namespace:           namespace,
					PolicyConfiguration: "ec-policy",
					PublicKey:           fmt.Sprintf("k8s://%s/%s", namespace, publicSecretName),
					Strict:              true,
					EffectiveTime:       "now",
					IgnoreRekor:         true,
				}
				generator.WithComponentImage(imageWithDigest)

				baselinePolicies := contract.PolicySpecWithSourceConfig(
					defaultECP.Spec, ecp.SourceConfig{Include: []string{"slsa_provenance_available"}})
				gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdatePolicyConfiguration(namespace, baselinePolicies)).To(gomega.Succeed())
			})

			ginkgo.It("succeeds when policy is met", func() {
				pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(generator, namespace, pipelineRunTimeout)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())

				pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "verify-enterprise-contract")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				printTaskRunStatus(tr, namespace, *fwk.AsKubeAdmin.CommonController)
				ginkgo.GinkgoWriter.Printf("Make sure TaskRun %s of PipelineRun %s succeeded\n", tr.PipelineTaskName, pr.Name)
				gomega.Expect(tekton.DidTaskRunSucceed(tr)).To(gomega.BeTrue())
				gomega.Expect(tr.Status.Results).Should(gomega.Or(
					gomega.ContainElements(tekton.MatchTaskRunResultWithJSONPathValue(constants.TektonTaskTestOutputName, "{$.result}", `["SUCCESS"]`)),
				))
			})

			ginkgo.It("does not pass when tests are not satisfied on non-strict mode", func() {
				policy := contract.PolicySpecWithSourceConfig(
					defaultECP.Spec, ecp.SourceConfig{Include: []string{"test"}})
				gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdatePolicyConfiguration(namespace, policy)).To(gomega.Succeed())
				generator.Strict = false
				pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(generator, namespace, pipelineRunTimeout)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())

				pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "verify-enterprise-contract")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				printTaskRunStatus(tr, namespace, *fwk.AsKubeAdmin.CommonController)
				gomega.Expect(tekton.DidTaskRunSucceed(tr)).To(gomega.BeTrue())
				gomega.Expect(tr.Status.Results).Should(gomega.Or(
					gomega.ContainElements(tekton.MatchTaskRunResultWithJSONPathValue(constants.TektonTaskTestOutputName, "{$.result}", `["FAILURE"]`)),
				))
			})

			ginkgo.It("fails when tests are not satisfied on strict mode", func() {
				policy := contract.PolicySpecWithSourceConfig(
					defaultECP.Spec, ecp.SourceConfig{Include: []string{"test"}})
				gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdatePolicyConfiguration(namespace, policy)).To(gomega.Succeed())

				generator.Strict = true
				pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(generator, namespace, pipelineRunTimeout)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())

				pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "verify-enterprise-contract")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				printTaskRunStatus(tr, namespace, *fwk.AsKubeAdmin.CommonController)
				gomega.Expect(tekton.DidTaskRunSucceed(tr)).To(gomega.BeFalse())
			})

			ginkgo.It("fails when unexpected signature is used", func() {
				secretName := fmt.Sprintf("dummy-public-key-%s", util.GenerateRandomString(10))
				publicKey := []byte("-----BEGIN PUBLIC KEY-----\n" +
					"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAENZxkE/d0fKvJ51dXHQmxXaRMTtVz\n" +
					"BQWcmJD/7pcMDEmBcmk8O1yUPIiFj5TMZqabjS9CQQN+jKHG+Bfi0BYlHg==\n" +
					"-----END PUBLIC KEY-----")
				gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdateSigningSecret(publicKey, secretName, namespace)).To(gomega.Succeed())
				generator.PublicKey = fmt.Sprintf("k8s://%s/%s", namespace, secretName)

				pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(generator, namespace, pipelineRunTimeout)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())

				pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "verify-enterprise-contract")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				printTaskRunStatus(tr, namespace, *fwk.AsKubeAdmin.CommonController)
				gomega.Expect(tekton.DidTaskRunSucceed(tr)).To(gomega.BeFalse())
			})

			ginkgo.Context("ec-cli command", func() {
				ginkgo.It("verifies ec cli has error handling", func() {
					generator.WithComponentImage("quay.io/konflux-ci/ec-golden-image:latest")
					pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(generator, namespace, pipelineRunTimeout)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())

					pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())

					tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "verify-enterprise-contract")
					gomega.Expect(err).NotTo(gomega.HaveOccurred())

					gomega.Expect(tr.Status.Results).Should(gomega.Or(
						gomega.ContainElements(tekton.MatchTaskRunResultWithJSONPathValue(constants.TektonTaskTestOutputName, "{$.result}", `["FAILURE"]`)),
					))
					reportLog, err := framework.GetContainerLogs(fwk.AsKubeAdmin.CommonController.KubeInterface(), tr.Status.PodName, "step-report-json", namespace)
					ginkgo.GinkgoWriter.Printf("*** Logs from pod '%s', container '%s':\n----- START -----%s----- END -----\n", tr.Status.PodName, "step-report-json", reportLog)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					gomega.Expect(reportLog).Should(gomega.ContainSubstring("No image attestations found matching the given public key"))
				})

				ginkgo.It("verifies ec validate accepts a list of image references", func() {
					secretName := fmt.Sprintf("golden-image-public-key%s", util.GenerateRandomString(10))
					goldenImagePublicKey := []byte("-----BEGIN PUBLIC KEY-----\n" +
						"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEZP/0htjhVt2y0ohjgtIIgICOtQtA\n" +
						"naYJRuLprwIv6FDhZ5yFjYUEtsmoNcW7rx2KM6FOXGsCX3BNc7qhHELT+g==\n" +
						"-----END PUBLIC KEY-----")
					gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdateSigningSecret(goldenImagePublicKey, secretName, namespace)).To(gomega.Succeed())
					generator.PublicKey = fmt.Sprintf("k8s://%s/%s", namespace, secretName)

					policy := contract.PolicySpecWithSource(
						defaultECP.Spec,
						ecp.Source{
							Config: &ecp.SourceConfig{
								Include: []string{"@slsa3"},
								Exclude: []string{"slsa_source_correlated.source_code_reference_provided"},
							},
							RuleData: &apiextensionsv1.JSON{Raw: []byte(`{"restrict_cve_security_levels": ["critical"]}`)},
						})
					gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdatePolicyConfiguration(namespace, policy)).To(gomega.Succeed())

					generator.WithComponentImage("quay.io/konflux-ci/ec-golden-image:latest")
					generator.AppendComponentImage("quay.io/konflux-ci/ec-golden-image:e2e-test-unacceptable-task")
					pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(generator, namespace, pipelineRunTimeout)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())

					pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())

					tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "verify-enterprise-contract")
					gomega.Expect(err).NotTo(gomega.HaveOccurred())

					gomega.Expect(tr.Status.Results).Should(gomega.Or(
						gomega.ContainElements(tekton.MatchTaskRunResultWithJSONPathValue(constants.TektonTaskTestOutputName, "{$.result}", `["SUCCESS"]`)),
					))
					reportLog, err := framework.GetContainerLogs(fwk.AsKubeAdmin.CommonController.KubeInterface(), tr.Status.PodName, "step-report-json", namespace)
					ginkgo.GinkgoWriter.Printf("*** Logs from pod '%s', container '%s':\n----- START -----%s----- END -----\n", tr.Status.PodName, "step-report-json", reportLog)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
				})
			})

			ginkgo.Context("Release Policy", func() {
				ginkgo.It("verifies redhat products pass the redhat policy rule collection before release", func() {
					secretName := fmt.Sprintf("golden-image-public-key%s", util.GenerateRandomString(10))
					goldenImagePublicKey := []byte("-----BEGIN PUBLIC KEY-----\n" +
						"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEZP/0htjhVt2y0ohjgtIIgICOtQtA\n" +
						"naYJRuLprwIv6FDhZ5yFjYUEtsmoNcW7rx2KM6FOXGsCX3BNc7qhHELT+g==\n" +
						"-----END PUBLIC KEY-----")
					gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdateSigningSecret(goldenImagePublicKey, secretName, namespace)).To(gomega.Succeed())
					generator.PublicKey = fmt.Sprintf("k8s://%s/%s", namespace, secretName)
					// Append extra excludes to the default ECP rather than replacing
					// the entire config, which would drop its existing excludes.
					releasePolicy := *defaultECP.Spec.DeepCopy()
					for i := range releasePolicy.Sources {
						releasePolicy.Sources[i].Config.Exclude = append(
							releasePolicy.Sources[i].Config.Exclude,
							"slsa_source_correlated.source_code_reference_provided",
							"cve.cve_results_found",
						)
						releasePolicy.Sources[i].RuleData = &apiextensionsv1.JSON{Raw: []byte(`{"restrict_cve_security_levels": ["critical"]}`)}
					}
					gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdatePolicyConfiguration(namespace, releasePolicy)).To(gomega.Succeed())

					generator.WithComponentImage("quay.io/konflux-ci/ec-golden-image:latest")
					pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(generator, namespace, pipelineRunTimeout)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())

					pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())

					tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "verify-enterprise-contract")
					gomega.Expect(err).NotTo(gomega.HaveOccurred())

					gomega.Expect(tr.Status.Results).ShouldNot(gomega.Or(
						gomega.ContainElements(tekton.MatchTaskRunResultWithJSONPathValue(constants.TektonTaskTestOutputName, "{$.result}", `["FAILURE"]`)),
					))
				})

				ginkgo.It("verifies the release policy: Task are trusted", func() {
					secretName := fmt.Sprintf("golden-image-public-key%s", util.GenerateRandomString(10))
					goldenImagePublicKey := []byte("-----BEGIN PUBLIC KEY-----\n" +
						"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEZP/0htjhVt2y0ohjgtIIgICOtQtA\n" +
						"naYJRuLprwIv6FDhZ5yFjYUEtsmoNcW7rx2KM6FOXGsCX3BNc7qhHELT+g==\n" +
						"-----END PUBLIC KEY-----")
					gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdateSigningSecret(goldenImagePublicKey, secretName, namespace)).To(gomega.Succeed())
					generator.PublicKey = fmt.Sprintf("k8s://%s/%s", namespace, secretName)
					policy := contract.PolicySpecWithSourceConfig(
						defaultECP.Spec,
						ecp.SourceConfig{Include: []string{"trusted_task.trusted"}},
					)
					gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdatePolicyConfiguration(namespace, policy)).To(gomega.Succeed())

					generator.WithComponentImage("quay.io/konflux-ci/ec-golden-image:e2e-test-unacceptable-task")
					pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(generator, namespace, pipelineRunTimeout)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())

					pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())

					tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "verify-enterprise-contract")
					gomega.Expect(err).NotTo(gomega.HaveOccurred())

					gomega.Expect(tr.Status.Results).Should(gomega.Or(
						gomega.ContainElements(tekton.MatchTaskRunResultWithJSONPathValue(constants.TektonTaskTestOutputName, "{$.result}", `["FAILURE"]`)),
					))

					reportLog, err := framework.GetContainerLogs(fwk.AsKubeAdmin.CommonController.KubeInterface(), tr.Status.PodName, "step-report-json", namespace)
					ginkgo.GinkgoWriter.Printf("*** Logs from pod '%s', container '%s':\n----- START -----%s----- END -----\n", tr.Status.PodName, "step-report-json", reportLog)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					gomega.Expect(reportLog).Should(gomega.MatchRegexp(`PipelineTask .* uses an untrusted task reference`))
				})

				ginkgo.It("verifies the release policy: Task references are pinned", func() {
					secretName := fmt.Sprintf("unpinned-task-bundle-public-key%s", util.GenerateRandomString(10))
					unpinnedTaskPublicKey := []byte("-----BEGIN PUBLIC KEY-----\n" +
						"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEPfwkY/ru2JRd6FSqIp7lT3gzjaEC\n" +
						"EAg+paWtlme2KNcostCsmIbwz+bc2aFV+AxCOpRjRpp3vYrbS5KhkmgC1Q==\n" +
						"-----END PUBLIC KEY-----")
					gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdateSigningSecret(unpinnedTaskPublicKey, secretName, namespace)).To(gomega.Succeed())
					generator.PublicKey = fmt.Sprintf("k8s://%s/%s", namespace, secretName)

					policy := contract.PolicySpecWithSourceConfig(
						defaultECP.Spec,
						ecp.SourceConfig{Include: []string{"trusted_task.pinned"}},
					)
					gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdatePolicyConfiguration(namespace, policy)).To(gomega.Succeed())

					generator.WithComponentImage("quay.io/redhat-appstudio-qe/enterprise-contract-tests:e2e-test-unpinned-task-bundle")
					pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(generator, namespace, pipelineRunTimeout)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())

					pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())

					tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "verify-enterprise-contract")
					gomega.Expect(err).NotTo(gomega.HaveOccurred())

					gomega.Expect(tr.Status.Results).Should(gomega.Or(
						gomega.ContainElements(tekton.MatchTaskRunResultWithJSONPathValue(constants.TektonTaskTestOutputName, "{$.result}", `["WARNING"]`)),
					))

					reportLog, err := framework.GetContainerLogs(fwk.AsKubeAdmin.CommonController.KubeInterface(), tr.Status.PodName, "step-report-json", namespace)
					ginkgo.GinkgoWriter.Printf("*** Logs from pod '%s', container '%s':\n----- START -----%s----- END -----\n", tr.Status.PodName, "step-report-json", reportLog)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					gomega.Expect(reportLog).Should(gomega.MatchRegexp(`Pipeline task .* uses an unpinned task reference`))
				})
			})
		})

	})
})

func printTaskRunStatus(tr *pipeline.PipelineRunTaskRunStatus, namespace string, sc common.SuiteController) {
	if tr.Status == nil {
		ginkgo.GinkgoWriter.Println("*** TaskRun status: nil")
		return
	}

	if y, err := yaml.Marshal(tr.Status); err == nil {
		ginkgo.GinkgoWriter.Printf("*** TaskRun status:\n%s\n", string(y))
	} else {
		ginkgo.GinkgoWriter.Printf("*** Unable to serialize TaskRunStatus to YAML: %#v; error: %s\n", tr.Status, err)
	}

	for _, s := range tr.Status.Steps {
		if logs, err := framework.GetContainerLogs(sc.KubeInterface(), tr.Status.PodName, s.Container, namespace); err == nil {
			ginkgo.GinkgoWriter.Printf("*** Logs from pod '%s', container '%s':\n----- START -----%s----- END -----\n", tr.Status.PodName, s.Container, logs)
		} else {
			ginkgo.GinkgoWriter.Printf("*** Can't fetch logs from pod '%s', container '%s': %s\n", tr.Status.PodName, s.Container, err)
		}
	}
}
