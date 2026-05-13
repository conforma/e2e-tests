package contract

import (
	ecp "github.com/conforma/crds/api/v1alpha1"
)

func PolicySpecWithSourceConfig(spec ecp.EnterpriseContractPolicySpec, sourceConfig ecp.SourceConfig) ecp.EnterpriseContractPolicySpec {
	var sources []ecp.Source
	for _, s := range spec.Sources {
		source := s.DeepCopy()
		source.Config = sourceConfig.DeepCopy()
		sources = append(sources, *source)
	}

	newSpec := *spec.DeepCopy()
	newSpec.Sources = sources
	return newSpec
}

func PolicySpecWithSource(spec ecp.EnterpriseContractPolicySpec, ecpSource ecp.Source) ecp.EnterpriseContractPolicySpec {
	var sources []ecp.Source
	for _, s := range spec.Sources {
		source := s.DeepCopy()
		source.Config = ecpSource.Config
		source.RuleData = ecpSource.RuleData
		sources = append(sources, *source)
	}

	newSpec := *spec.DeepCopy()
	newSpec.Sources = sources
	return newSpec
}
