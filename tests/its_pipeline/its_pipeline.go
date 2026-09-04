package its_pipeline

import (
	"fmt"
	"os"
	"time"

	ecp "github.com/conforma/crds/api/v1alpha1"
	"github.com/conforma/e2e-tests/pkg/constants"
	"github.com/conforma/e2e-tests/pkg/framework"
	"github.com/conforma/e2e-tests/pkg/utils/contract"
	"github.com/conforma/e2e-tests/pkg/utils/tekton"
	"github.com/devfile/library/v2/pkg/util"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
)

var _ = framework.ConformaSuiteDescribe("ITS Pipeline E2E tests", ginkgo.Label("its-pipeline"), func() {

	defer ginkgo.GinkgoRecover()

	var namespace string
	var fwk *framework.Framework
	var imageWithDigest string
	var pipelineRunTimeout int
	var defaultECP *ecp.EnterpriseContractPolicy
	var itsRepoURL, itsRevision, itsPath string

	ginkgo.AfterEach(framework.ReportFailure(&fwk))

	ginkgo.BeforeAll(func() {
		var err error
		fwk, err = framework.NewFramework(framework.GetGeneratedNamespace(constants.TEKTON_CHAINS_E2E_USER))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(fwk.UserNamespace).NotTo(gomega.BeEmpty(), "failed to create sandbox user")
		namespace = fwk.UserNamespace

		pipelineRunTimeout = int(time.Duration(20) * time.Minute)

		// ITS pipeline location, configurable via env vars for testing PR branches
		itsRepoURL = os.Getenv(constants.ITS_PIPELINE_REPO_URL_ENV)
		if itsRepoURL == "" {
			itsRepoURL = constants.ITSPipelineRepoURLDefault
		}
		itsRevision = os.Getenv(constants.ITS_PIPELINE_REVISION_ENV)
		if itsRevision == "" {
			itsRevision = constants.ITSPipelineRevisionDefault
		}
		itsPath = os.Getenv(constants.ITS_PIPELINE_PATH_ENV)
		if itsPath == "" {
			itsPath = constants.ITSPipelinePathDefault
		}
		ginkgo.GinkgoWriter.Printf("ITS pipeline location: %s @ %s path=%s\n", itsRepoURL, itsRevision, itsPath)

		// Build and sign an image via docker-build pipeline
		buildPipelineRunName := fmt.Sprintf("buildah-demo-%s", util.GenerateRandomString(10))
		image := fmt.Sprintf("quay.io/%s/test-images:%s", framework.GetQuayIOOrganization(), buildPipelineRunName)

		gomega.Expect(fwk.AsKubeAdmin.CommonController.CreateQuayRegistrySecret(namespace)).To(gomega.Succeed())

		bundles, err := fwk.AsKubeAdmin.TektonController.NewBundles()
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		dockerBuildBundle := bundles.DockerBuildOCITAMinBundle
		dockerBuildPipelineName := "docker-build-oci-ta-min"
		if dockerBuildBundle == "" {
			dockerBuildBundle = bundles.DockerBuildBundle
			dockerBuildPipelineName = "docker-build"
		}
		gomega.Expect(dockerBuildBundle).NotTo(gomega.Equal(""), "Can't continue without a docker-build pipeline got from selector config")

		pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(tekton.BuildahDemo{
			Image: image, Bundle: dockerBuildBundle, PipelineName: dockerBuildPipelineName,
			Namespace: namespace, Name: buildPipelineRunName,
		}, namespace, pipelineRunTimeout)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())
		ginkgo.GinkgoWriter.Printf("Build pipeline %q in namespace %q succeeded\n", pr.Name, pr.Namespace)

		pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		digest, err := fwk.AsKubeAdmin.TektonController.GetTaskRunResult(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "build-container", "IMAGE_DIGEST")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		imageURL, err := fwk.AsKubeAdmin.TektonController.GetTaskRunResult(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "build-container", "IMAGE_URL")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(imageURL).To(gomega.Equal(image))

		imageWithDigest = fmt.Sprintf("%s@%s", image, digest)
		ginkgo.GinkgoWriter.Printf("Built image: %s\n", imageWithDigest)

		// Wait for Chains to sign and attest the image
		err = fwk.AsKubeAdmin.TektonController.AwaitAttestationAndSignature(imageWithDigest, constants.ChainsAttestationTimeout)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(),
			"Could not find .att or .sig within the %s timeout. "+
				"The chains-controller likely did not create them in time.",
			constants.ChainsAttestationTimeout.String(),
		)
		ginkgo.GinkgoWriter.Printf("Chains attestation and signature confirmed for %s\n", imageWithDigest)

		// Set up default ECP matching Red Hat Konflux defaults
		defaultECP = &ecp.EnterpriseContractPolicy{}
		defaultECP.Spec = ecp.EnterpriseContractPolicySpec{
			Name:        "Red Hat",
			Description: "Includes the full set of rules and policies required internally by Red Hat when building Red Hat products.",
			Sources: []ecp.Source{
				{
					Name: "Default",
					Policy: []string{
						"oci::quay.io/conforma/release-policy:latest",
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
	})

	ginkgo.Context("enterprise-contract ITS pipeline", ginkgo.Label("pipeline"), func() {
		var generator tekton.ITSPipeline

		ginkgo.BeforeEach(func() {
			generator = tekton.ITSPipeline{
				Name:                "its-pipeline",
				Namespace:           namespace,
				PolicyConfiguration: "ec-policy",
				PublicKey:           "k8s://openshift-pipelines/public-key",
				Strict:              true,
				EffectiveTime:       "now",
				RepoURL:             itsRepoURL,
				Revision:            itsRevision,
				PathInRepo:          itsPath,
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

			tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "verify")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(tekton.DidTaskRunSucceed(tr)).To(gomega.BeTrue())
			gomega.Expect(tr.Status.Results).Should(
				gomega.ContainElements(tekton.MatchTaskRunResultWithJSONPathValue(constants.TektonTaskTestOutputName, "{$.result}", `["SUCCESS"]`)),
			)
		})

		ginkgo.It("fails when policy is not met in strict mode", func() {
			policy := contract.PolicySpecWithSourceConfig(
				defaultECP.Spec, ecp.SourceConfig{Include: []string{"test"}})
			gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdatePolicyConfiguration(namespace, policy)).To(gomega.Succeed())

			generator.Strict = true
			pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(generator, namespace, pipelineRunTimeout)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())

			pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "verify")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(tekton.DidTaskRunSucceed(tr)).To(gomega.BeFalse())
		})

		ginkgo.It("reports failure but does not fail in non-strict mode", func() {
			policy := contract.PolicySpecWithSourceConfig(
				defaultECP.Spec, ecp.SourceConfig{Include: []string{"test"}})
			gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdatePolicyConfiguration(namespace, policy)).To(gomega.Succeed())

			generator.Strict = false
			pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(generator, namespace, pipelineRunTimeout)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())

			pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "verify")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(tekton.DidTaskRunSucceed(tr)).To(gomega.BeTrue())
			gomega.Expect(tr.Status.Results).Should(
				gomega.ContainElements(tekton.MatchTaskRunResultWithJSONPathValue(constants.TektonTaskTestOutputName, "{$.result}", `["FAILURE"]`)),
			)
		})
	})
})
