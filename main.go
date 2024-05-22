package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type serverAvg struct {
	Server  string
	AvgTime float64
}

func main() {
	var fileName, outputFile string
	var topN int
	flag.StringVar(&fileName, "f", "", "Path to the file containing DNS server IPs")
	flag.StringVar(&outputFile, "o", "", "Output file to save the IPs (optional)")
	flag.IntVar(&topN, "n", -1, "Number of top fastest DNS servers to display (optional, -1 for immediate print)")
	flag.Parse()

	if fileName == "" {
		fmt.Println("Please specify the file with -f flag.")
		flag.Usage()
		os.Exit(1)
	}

	file, err := os.Open(fileName)
	if err != nil {
		fmt.Printf("Failed to open file: %s\n", err)
		os.Exit(1)
	}
	defer file.Close()

	var servers []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "#")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				servers = append(servers, ip)
			}
		}
	}

	results := make(chan serverAvg)
	var wg sync.WaitGroup

	for _, server := range servers {
		wg.Add(1)
		go func(server string) {
			defer wg.Done()
			avgTime := testDNS(server)
			if avgTime < 1.0 { // Only consider results under 1 second
				results <- serverAvg{Server: server, AvgTime: avgTime}
			}
		}(server)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var allResults []serverAvg
	for result := range results {
		allResults = append(allResults, result)
	}

	if topN > 0 {
		sort.Slice(allResults, func(i, j int) bool {
			return allResults[i].AvgTime < allResults[j].AvgTime
		})
		if topN < len(allResults) {
			allResults = allResults[:topN]
		}
	}

	printResults(allResults, outputFile)
}

func testDNS(server string) float64 {
	domains := []string{"www.google.com.", "youtube.com.", "facebook.com."}
	var totalDuration time.Duration
	for _, domain := range domains {
		c := new(dns.Client)
		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn(domain), dns.TypeA)
		start := time.Now()
		_, _, err := c.Exchange(m, server+":53")
		duration := time.Since(start)
		if err != nil || duration >= time.Second {
			return float64(time.Second) // Ignore DNS queries taking longer than 1 second
		}
		totalDuration += duration
	}
	return float64(totalDuration) / float64(len(domains)) / float64(time.Second)
}

func printResults(results []serverAvg, outputFile string) {
	var out *os.File = os.Stdout
	if outputFile != "" {
		var err error
		out, err = os.Create(outputFile)
		if err != nil {
			fmt.Printf("Error opening output file: %s\n", err)
			os.Exit(1)
		}
		defer out.Close()

		for _, result := range results {
			fmt.Fprintln(out, result.Server) // Only write IP address
		}
	} else {
		for _, result := range results {
			fmt.Printf("%-15s %.2fms\n", result.Server, result.AvgTime*1000)
		}
	}
}
