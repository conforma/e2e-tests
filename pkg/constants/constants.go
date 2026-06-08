package constants

import "time"

const (
	TEKTON_CHAINS_E2E_USER         = "chains-e2e"
	TEKTON_CHAINS_SIGNING_SECRETS_NAME = "signing-secrets"
	TektonTaskTestOutputName       = "TEST_OUTPUT"
	DefaultPipelineServiceAccount  = "konflux-integration-runner"

	PipelineRunPollingInterval = 20 * time.Second
	ChainsAttestationTimeout   = 20 * time.Minute

	QuayRepositorySecretName      = "quay-repository"
	QuayRepositorySecretNamespace = "e2e-secrets"

	E2E_APPLICATIONS_NAMESPACE_ENV = "E2E_APPLICATIONS_NAMESPACE"
	QUAY_E2E_ORGANIZATION_ENV      = "QUAY_E2E_ORGANIZATION_ENV"

	ArgoCDLabelKey   = "argocd.argoproj.io/managed-by"
	ArgoCDLabelValue = "gitops-service-argocd"
	TenantLabelKey   = "konflux-ci.dev/type"
	TenantLabelValue = "tenant"
	WorkspaceLabelKey = "appstudio.redhat.com/workspace_name"

	TEST_ENVIRONMENT_ENV    = "TEST_ENVIRONMENT"
	UpstreamTestEnvironment = "upstream"

	KonfluxAdminUserActionsClusterRoleName = "konflux-admin-user-actions"
	DefaultKonfluxAdminRoleBindingName     = "user2-konflux-admin"
	DefaultKonfluxCIUserName               = "user2@konflux.dev"
	DefaultPipelineSARoleBindingName       = "pipeline-sa-admin"
)
