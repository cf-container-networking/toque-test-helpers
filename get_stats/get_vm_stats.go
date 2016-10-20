package get_stats

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const Minutes = 60

var _ = Describe("gathering cpu stats for vms", func() {
	Describe("networking policy", func() {
		It("something", func(done Done) {

			stats := "timestamp,load (1m),load (5m),load (15m),cpu usr,cpu sys,cpu wait\n"
			for i := 1; ; i++ {
				fmt.Printf("getting stats %d out of %d\n", i, Minutes)
				stats = fmt.Sprintf("%s%s", stats, GetVMCPUUsage())
				if i == Minutes {
					break
				}
				time.Sleep(1 * time.Minute)
			}

			file, err := os.Create("stats/toque_stats.csv")
			Expect(err).NotTo(HaveOccurred())
			defer file.Close()
			_, err = file.WriteString(stats)
			Expect(err).NotTo(HaveOccurred())

			close(done)
		}, (Minutes+30)*60 /* <-- overall spec timeout in seconds */)
	})
})

func GetVMCPUUsage() string {
	now := time.Now()
	cmd := exec.Command("bosh", "vms", "--vitals")
	output, err := cmd.Output()
	Expect(err).NotTo(HaveOccurred())

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "cell") {
			fields := strings.Split(line, "|")
			load := strings.Split(fields[6], ",")
			return fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s\n",
				now.Format("2006-01-02 15:04:05"),
				strings.TrimSpace(load[0]),
				strings.TrimSpace(load[1]),
				strings.TrimSpace(load[2]),
				strings.TrimSpace(fields[7]),
				strings.TrimSpace(fields[8]),
				strings.TrimSpace(fields[9]))
		}
	}
	return ""
}
