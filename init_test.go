package acceptance_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"lib/testsupport"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry-incubator/cf-test-helpers/helpers"
	. "github.com/onsi/ginkgo"
	ginkgoConfig "github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

var (
	appsDir    string
	config     helpers.Config
	testConfig struct {
		TestUser         string `json:"test_user"`
		TestUserPassword string `json:"test_user_password"`
		Applications     int    `json:"test_applications"`
		StartIndex       int    `json:"start_index"`
	}
	preBuiltBinaries map[string]string
)

func Auth(username, password string) {
	By("authenticating as " + username)
	cmd := exec.Command("cf", "auth", username, password)
	sess, err := gexec.Start(cmd, nil, nil)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess.Wait(Timeout_Short)).Should(gexec.Exit(0))
}

func AuthAsAdmin() {
	Auth(config.AdminUser, config.AdminPassword)
}

func preBuildLinuxBinary(appType string) {
	By("pre-building the linux binary for " + appType)
	os.Setenv("GOOS", "linux")
	os.Setenv("GOARCH", "amd64")
	appDir := filepath.Join(appsDir, appType)
	Expect(exec.Command("go", "build", "-o", filepath.Join(appDir, appType), appDir).Run()).To(Succeed())
}

func TestAcceptance(t *testing.T) {
	RegisterFailHandler(Fail)

	BeforeSuite(func() {
		config = helpers.LoadConfig()

		configPath := helpers.ConfigPath()
		configBytes, err := ioutil.ReadFile(configPath)
		Expect(err).NotTo(HaveOccurred())

		err = json.Unmarshal(configBytes, &testConfig)
		Expect(err).NotTo(HaveOccurred())

		if testConfig.Applications <= 0 {
			Fail("Applications count needs to be greater than 0")
		}

		Expect(cf.Cf("api", "--skip-ssl-validation", config.ApiEndpoint).Wait(Timeout_Short)).To(gexec.Exit(0))
		AuthAsAdmin()

		appsDir = os.Getenv("APPS_DIR")
		Expect(appsDir).NotTo(BeEmpty())

		preBuildLinuxBinary("proxy")

		rand.Seed(ginkgoConfig.GinkgoConfig.RandomSeed + int64(GinkgoParallelNode()))
	})

	AfterSuite(func() {
		// remove binaries
		Expect(os.Remove(filepath.Join(appsDir, "proxy", "proxy"))).To(Succeed())
	})

	RunSpecs(t, "Acceptance Suite")
}

func appDir(appType string) string {
	return filepath.Join(appsDir, appType)
}

func defaultManifest(appType string) string {
	return filepath.Join(appDir(appType), "manifest.yml")
}

func pushAppsOfType(appNames []string, appType string, manifest string) {
	By(fmt.Sprintf("pushing %d apps of type %s", len(appNames), appType))

	parallelRunner := &testsupport.ParallelRunner{
		NumWorkers: 16,
	}
	parallelRunner.RunOnSliceStrings(appNames, func(appName string) {
		pushAppOfType(appName, appType, manifest)
	})
}

func pushAppOfType(appName string, appType string, manifest string) {
	By(fmt.Sprintf("pushing app %s of type %s", appName, appType))
	Expect(cf.Cf(
		"push", appName,
		"-p", appDir(appType),
		"-f", manifest,
		"-c", "./"+appType,
		"-b", "binary_buildpack",
	).Wait(Timeout_Push)).To(gexec.Exit(0))
}

func pushRegistryApp(appName string) {
	Expect(cf.Cf(
		"push", appName,
		"-p", appDir("registry"),
		"-c", "./registry",
		"-b", "binary_buildpack",
		"-m", "32M",
	).Wait(Timeout_Push)).To(gexec.Exit(0))
}
