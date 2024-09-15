package main

import (
	"context"
	"fmt"
	"github.com/cli/go-gh/v2/pkg/auth"
	"log"
	"os"
	"sync"
	"time"

	"github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"
)

type Stats struct {
	Total           int
	AvgDurationSecs int
	MinDurationSecs int
	MaxDurationSecs int
}

func fetchDeployments(client *github.Client, owner, repo, environment string, maxPages int) ([]*github.Deployment, error) {
	var allDeployments []*github.Deployment
	ctx := context.Background()
	opts := &github.DeploymentsListOptions{
		Environment: environment,
		ListOptions: github.ListOptions{PerPage: 100}, // Start with the first page
	}

	pageCounter := 0

	for {
		// Fetch one page of deployments
		deployments, resp, err := client.Repositories.ListDeployments(ctx, owner, repo, opts)
		if err != nil {
			return nil, err
		}
		// Append the deployments from this page to the total list
		allDeployments = append(allDeployments, deployments...)

		// If there are no more pages or we've reached the max number of pages, break the loop
		pageCounter++
		if resp.NextPage == 0 || pageCounter >= maxPages {
			break
		}

		// Update the page number to fetch the next page
		opts.Page = resp.NextPage
	}

	return allDeployments, nil
}

func fetchSuccessStatus(client *github.Client, owner, repo string, deploymentID int64) (*github.DeploymentStatus, error) {
	ctx := context.Background()
	statuses, _, err := client.Repositories.ListDeploymentStatuses(ctx, owner, repo, deploymentID, nil)
	if err != nil {
		return nil, err
	}
	fmt.Printf(".")

	for _, status := range statuses {
		if status.GetState() == "success" {
			return status, nil
		}
	}
	return nil, nil
}

func fetchStatusesConcurrently(client *github.Client, owner, repo string, deployments []*github.Deployment, maxWorkers int) (map[int64]*github.DeploymentStatus, error) {
	statusCh := make(chan struct {
		ID     int64
		Status *github.DeploymentStatus
		Err    error
	}, len(deployments))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, maxWorkers)

	fmt.Printf("... fetching deployment statuses ")
	for _, deployment := range deployments {
		wg.Add(1)
		deployment := deployment
		go func() {
			defer wg.Done()
			semaphore <- struct{}{}
			status, err := fetchSuccessStatus(client, owner, repo, deployment.GetID())
			<-semaphore

			statusCh <- struct {
				ID     int64
				Status *github.DeploymentStatus
				Err    error
			}{
				ID:     deployment.GetID(),
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

	results := make(map[int64]*github.DeploymentStatus)
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

func computeStats(deployments []*github.Deployment, statuses map[int64]*github.DeploymentStatus) (Stats, error) {
	var durations []int
	var total, sum int

	for _, deployment := range deployments {
		successStatus, found := statuses[deployment.GetID()]
		if !found || successStatus == nil {
			continue
		}

		duration := durationSeconds(deployment.GetCreatedAt().Time, successStatus.GetCreatedAt().Time)
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

func run(client *github.Client, owner, repo, environment, cutoffISO8601 string) error {
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

		var oldDeployments, newDeployments []*github.Deployment
		for _, deployment := range deployments {
			if deployment.GetCreatedAt().Time.Before(cutoff) {
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

	// Get the authentication token from the GitHub CLI
	token, _ := auth.TokenForHost("github.com")
	if token == "" {
		log.Fatalf("Error getting GitHub auth token")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

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
