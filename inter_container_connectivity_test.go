package acceptance_test

import (
	"fmt"
	"io/ioutil"
	"lib/models"
	"lib/policy_client"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"code.cloudfoundry.org/lager/lagertest"

	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/pivotal-cf-experimental/rainmaker"
)

const Timeout_Push = 5 * time.Minute
const Timeout_Short = 10 * time.Second

var ports []int

func getSubnet(ip string) string {
	return strings.Split(ip, ".")[2]
}

func isSameCell(sourceIP, destIP string) bool {
	return getSubnet(sourceIP) == getSubnet(destIP)
}

var _ = Describe("connectivity between containers on the overlay network", func() {
	Describe("networking policy", func() {
		var (
			appsTest     []string
			orgName      string
			spaceName    string
			applications int
			start        int
		)

		BeforeEach(func() {
			applications = testConfig.Applications
			start = testConfig.StartIndex

			for i := start; i < applications+start; i++ {
				appsTest = append(appsTest, fmt.Sprintf("proxy-%d", i))
			}

			Auth(testConfig.TestUser, testConfig.TestUserPassword)

			orgName = "test-org"
			Expect(cf.Cf("create-org", orgName).Wait(Timeout_Push)).To(gexec.Exit(0))
			Expect(cf.Cf("target", "-o", orgName).Wait(Timeout_Push)).To(gexec.Exit(0))

			spaceName = "test-space"
			Expect(cf.Cf("create-space", spaceName).Wait(Timeout_Push)).To(gexec.Exit(0))
			Expect(cf.Cf("target", "-o", orgName, "-s", spaceName).Wait(Timeout_Push)).To(gexec.Exit(0))
		})

		It("allows the user to configure policies", func(done Done) {
			By("pushing the apps")
			runWithTimeout("push apps", 5*time.Minute, func() {
				pushAppsOfType(appsTest, "proxy", defaultManifest("proxy"))
			})

			policies := []int{4000}
			for _, nPolicies := range policies {
				By(fmt.Sprintf("creating %d policies", nPolicies))
				for i := 0; i < nPolicies; i++ {
					ports = append(ports, 7000+i)
				}
				doAllSelfPolicies("create", appsTest, ports)

				stats := fmt.Sprintf("Test Parameters: Apps %d\n", applications+start)
				By("test set up complete waiting 2 mins")
				time.Sleep(1 * time.Minute)
				stats = fmt.Sprintf("%sCell Stats: %s\n", stats, GetVMCPUUsage())
				time.Sleep(1 * time.Minute)
				By("1 minute...")
				stats = fmt.Sprintf("%sCell Stats: %s\n", stats, GetVMCPUUsage())
				time.Sleep(1 * time.Minute)
				By("2 minutes...")
				stats = fmt.Sprintf("%sCell Stats: %s\n", stats, GetVMCPUUsage())

				file, err := os.Create(fmt.Sprintf("stats/toque_test_policy_scaling_%d_policies", nPolicies))
				Expect(err).NotTo(HaveOccurred())
				defer file.Close()
				_, err = file.WriteString(stats)
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("deleting %d policies", nPolicies))
				doAllSelfPolicies("delete", appsTest, ports)
			}

			close(done)
		}, 30*60 /* <-- overall spec timeout in seconds */)
	})
})

func getToken() string {
	By("getting token")
	cmd := exec.Command("cf", "oauth-token")
	session, err := gexec.Start(cmd, nil, nil)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session.Wait(2 * Timeout_Short)).Should(gexec.Exit(0))
	rawOutput := string(session.Out.Contents())
	return strings.TrimSpace(strings.TrimPrefix(rawOutput, "bearer "))
}

func getGuids(appNames []string) []string {
	guids := []string{}
	token := getToken()
	appsClient := rainmaker.NewApplicationsService(rainmaker.Config{Host: "http://" + config.ApiEndpoint})

	appsList, err := appsClient.List(token)
	Expect(err).NotTo(HaveOccurred())

	for {
		for _, app := range appsList.Applications {
			for _, appName := range appNames {
				if app.Name == appName {
					guids = append(guids, app.GUID)
					break
				}
			}
		}
		if appsList.HasNextPage() {
			appsList, err = appsList.Next(token)
			Expect(err).NotTo(HaveOccurred())
		} else {
			break
		}
	}

	Expect(guids).To(HaveLen(len(appNames)))

	return guids
}

func doAllSelfPolicies(action string, apps []string, dstPorts []int) {
	if len(dstPorts) <= 0 {
		return
	}
	policyClient := policy_client.NewExternal(lagertest.NewTestLogger("test"), &http.Client{}, "http://"+config.ApiEndpoint)
	guids := getGuids(apps)
	policies := []models.Policy{}
	for _, guid := range guids {
		for _, port := range dstPorts {
			policies = append(policies, models.Policy{
				Source: models.Source{
					ID: guid,
				},
				Destination: models.Destination{
					ID:       guid,
					Port:     port,
					Protocol: "tcp",
				},
			})
		}
	}
	token := getToken()
	if action == "create" {
		Expect(policyClient.AddPolicies(token, policies)).To(Succeed())
	} else if action == "delete" {
		Expect(policyClient.DeletePolicies(token, policies)).To(Succeed())
	}
}

func runWithTimeout(operation string, timeout time.Duration, work func()) {
	done := make(chan bool)
	go func() {
		fmt.Printf("starting %s\n", operation)
		work()
		done <- true
	}()

	select {
	case <-done:
		fmt.Printf("completed %s\n", operation)
		return
	case <-time.After(timeout):
		Fail("timeout on " + operation)
	}
}

var httpClient = &http.Client{
	Transport: &http.Transport{
		DisableKeepAlives: true,
		Dial: (&net.Dialer{
			Timeout:   4 * time.Second,
			KeepAlive: 0,
		}).Dial,
	},
}

func httpGetBytes(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return respBytes, nil
}

func GetVMCPUUsage() string {
	cmd := exec.Command("bosh", "vms", "--vitals")
	output, err := cmd.Output()
	Expect(err).NotTo(HaveOccurred())

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "cell") {
			return line
		}
	}
	return ""
}
