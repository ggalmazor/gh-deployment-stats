package main

import (
	"context"
	"flag"
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

func fetchDeployments(client *github.Client, owner, repo, environment string, totalDeployments int) ([]*github.Deployment, error) {
	var allDeployments []*github.Deployment
	ctx := context.Background()

	perPage := 100
	if totalDeployments > 0 && totalDeployments < 100 {
		perPage = totalDeployments
	}

	opts := &github.DeploymentsListOptions{
		Environment: environment,
		ListOptions: github.ListOptions{PerPage: perPage},
	}

	deploymentCounter := 0

	for {
		deployments, resp, err := client.Repositories.ListDeployments(ctx, owner, repo, opts)
		if err != nil {
			return nil, err
		}
		allDeployments = append(allDeployments, deployments...)
		deploymentCounter += len(deployments)

		if resp.NextPage == 0 || (totalDeployments > 0 && deploymentCounter >= totalDeployments) {
			break
		}

		opts.Page = resp.NextPage
	}

	if totalDeployments > 0 && len(allDeployments) > totalDeployments {
		allDeployments = allDeployments[:totalDeployments]
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

func run(client *github.Client, owner, repo, environment, cutoffISO8601 string, totalDeployments int) error {
	deployments, err := fetchDeployments(client, owner, repo, environment, totalDeployments)
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
func customUsage() {
	fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [-deployments] [-cutoff] <owner> <repo> <environment>\n", os.Args[0])
	fmt.Println("\nPositional Arguments:")
	fmt.Println("  owner         GitHub repository owner (required)")
	fmt.Println("  repo          GitHub repository name (required)")
	fmt.Println("  environment   Deployment environment (required)")
	fmt.Println("\nOptions:")
	flag.PrintDefaults() // Print all the optional flags
}

func main() {
	cutoffISO8601 := flag.String("cutoff", "", "Cutoff timestamp in ISO8601 format to divide results in two groups (optional)")
	totalDeployments := flag.Int("deployments", 500, "Total number of deployments to consider (optional)")
	flag.Usage = customUsage
	flag.Parse()

	if len(flag.Args()) < 3 {
		customUsage()
		os.Exit(1)
	}

	owner := flag.Arg(0)
	repo := flag.Arg(1)
	environment := flag.Arg(2)

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

	// Run the main logic
	if err := run(client, owner, repo, environment, *cutoffISO8601, *totalDeployments); err != nil {
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
