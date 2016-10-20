package get_stats

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestInit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Get Stats")
}
