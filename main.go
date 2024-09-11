package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

type Deployment struct {
	ID        int       `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type Status struct {
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

type Stats struct {
	Total           int
	AvgDurationSecs int
	MinDurationSecs int
	MaxDurationSecs int
}

func fetchDeployments(client *api.RESTClient, owner, repo, environment string, totalPages int) ([]Deployment, error) {
	var allDeployments []Deployment
	for page := 1; page <= totalPages; page++ {
		path := fmt.Sprintf("repos/%s/%s/deployments?environment=%s&per_page=100&page=%d", owner, repo, environment, page)
		var deployments []Deployment
		err := client.Get(path, &deployments)
		if err != nil {
			return nil, err
		}
		allDeployments = append(allDeployments, deployments...)
	}

	return allDeployments, nil
}

func fetchSuccessStatus(client *api.RESTClient, owner, repo string, deploymentID int) (*Status, error) {
	path := fmt.Sprintf("repos/%s/%s/deployments/%d/statuses", owner, repo, deploymentID)
	var statuses []Status
	err := client.Get(path, &statuses)
	if err != nil {
		return nil, err
	}
	fmt.Printf(".")

	for _, status := range statuses {
		if status.State == "success" {
			return &status, nil
		}
	}
	return nil, nil
}

func fetchStatusesConcurrently(client *api.RESTClient, owner, repo string, deployments []Deployment, maxWorkers int) (map[int]*Status, error) {
	statusCh := make(chan struct {
		ID     int
		Status *Status
		Err    error
	}, len(deployments))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, maxWorkers)

	fmt.Printf("... fetching deployment statues ")
	for _, deployment := range deployments {
		wg.Add(1)
		deployment := deployment
		go func() {
			defer wg.Done()
			semaphore <- struct{}{}
			status, err := fetchSuccessStatus(client, owner, repo, deployment.ID)
			<-semaphore

			statusCh <- struct {
				ID     int
				Status *Status
				Err    error
			}{
				ID:     deployment.ID,
				Status: status,
				Err:    err,
			}
		}()
	}

	go func() {
		wg.Wait()
		close(statusCh)
		fmt.Printf(" Done\n")
	}()

	results := make(map[int]*Status)
	for result := range statusCh {
		if result.Err != nil {
			return nil, result.Err
		}
		results[result.ID] = result.Status
	}

	return results, nil
}

func durationSeconds(a, b time.Time) int {
	return int(b.Sub(a).Seconds())
}

func computeStats(deployments []Deployment, statuses map[int]*Status) (Stats, error) {
	var durations []int
	var total, sum int

	for _, deployment := range deployments {
		successStatus, found := statuses[deployment.ID]
		if !found || successStatus == nil {
			continue
		}

		duration := durationSeconds(deployment.CreatedAt, successStatus.CreatedAt)
		if duration > 0 {
			durations = append(durations, duration)
		}
	}

	if len(durations) > 0 {
		total = len(durations)
		for _, duration := range durations {
			sum += duration
		}
		return Stats{
			Total:           total,
			AvgDurationSecs: sum / total,
			MinDurationSecs: minFromSlice(durations),
			MaxDurationSecs: maxFromSlice(durations),
		}, nil
	}

	return Stats{}, nil
}

func printStats(group string, stats Stats) {
	groupMessage := ""
	if group != "" {
		groupMessage = group + " "
	}
	fmt.Printf("- %d %ssuccessful deployments: avg %d secs, min/max: %d/%d secs\n",
		stats.Total, groupMessage, stats.AvgDurationSecs, stats.MinDurationSecs, stats.MaxDurationSecs)
}

func run(client *api.RESTClient, owner, repo, environment, cutoffISO8601 string) error {
	deployments, err := fetchDeployments(client, owner, repo, environment, 5)
	if err != nil {
		return err
	}

	fmt.Printf("Fetched %d deployments for %s:\n", len(deployments), environment)

	statuses, err := fetchStatusesConcurrently(client, owner, repo, deployments, 10)
	if err != nil {
		return err
	}

	if cutoffISO8601 != "" {
		cutoff, err := time.Parse(time.RFC3339, cutoffISO8601)
		if err != nil {
			return fmt.Errorf("invalid cutoff date: %w", err)
		}

		var oldDeployments, newDeployments []Deployment
		for _, deployment := range deployments {
			if deployment.CreatedAt.Before(cutoff) {
				oldDeployments = append(oldDeployments, deployment)
			} else {
				newDeployments = append(newDeployments, deployment)
			}
		}

		oldStats, err := computeStats(oldDeployments, statuses)
		if err != nil {
			return err
		}
		newStats, err := computeStats(newDeployments, statuses)
		if err != nil {
			return err
		}

		printStats("old", oldStats)
		printStats("new", newStats)
	} else {
		stats, err := computeStats(deployments, statuses)
		if err != nil {
			return err
		}
		printStats("", stats)
	}

	return nil
}

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: <owner> <repo> <environment> [cutoffISO8601]")
		os.Exit(1)
	}

	owner, repo, environment := os.Args[1], os.Args[2], os.Args[3]
	cutoffISO8601 := ""
	if len(os.Args) >= 5 {
		cutoffISO8601 = os.Args[4]
	}

	client, err := api.DefaultRESTClient()
	if err != nil {
		log.Fatalf("Failed to create GitHub client: %v", err)
	}

	if err := run(client, owner, repo, environment, cutoffISO8601); err != nil {
		log.Fatal(err)
	}
}

func minFromSlice(arr []int) int {
	minVal := arr[0]
	for _, v := range arr {
		if v < minVal {
			minVal = v
		}
	}
	return minVal
}

func maxFromSlice(arr []int) int {
	maxVal := arr[0]
	for _, v := range arr {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}
