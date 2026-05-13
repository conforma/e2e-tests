package framework

import (
	ginkgo "github.com/onsi/ginkgo/v2"
)

func ConformaSuiteDescribe(text string, args ...interface{}) bool {
	args = append(args, ginkgo.Ordered)
	return ginkgo.Describe("[conforma-suite "+text+"]", args...)
}
