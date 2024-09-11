package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
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

func requestGithubAPI(path string, token string) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.github.com%s", path), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("User-Agent", "Go")
	req.Header.Add("Authorization", fmt.Sprintf("token %s", token))
	req.Header.Add("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.New("GitHub API request failed")
	}

	return ioutil.ReadAll(resp.Body)
}

func fetchDeployments(owner, repo, environment, token string, totalPages int) ([]Deployment, error) {
	var allDeployments []Deployment

	for page := 1; page <= totalPages; page++ {
		path := fmt.Sprintf("/repos/%s/%s/deployments?environment=%s&per_page=100&page=%d", owner, repo, environment, page)
		body, err := requestGithubAPI(path, token)
		if err != nil {
			return nil, err
		}

		var deployments []Deployment
		if err := json.Unmarshal(body, &deployments); err != nil {
			return nil, err
		}

		allDeployments = append(allDeployments, deployments...)
	}

	return allDeployments, nil
}

func fetchSuccessStatus(owner, repo, token string, deploymentID int) (*Status, error) {
	path := fmt.Sprintf("/repos/%s/%s/deployments/%d/statuses", owner, repo, deploymentID)
	body, err := requestGithubAPI(path, token)
	if err != nil {
		return nil, err
	}

	var statuses []Status
	if err := json.Unmarshal(body, &statuses); err != nil {
		return nil, err
	}

	for _, status := range statuses {
		if status.State == "success" {
			return &status, nil
		}
	}

	return nil, nil
}

func durationSeconds(a, b time.Time) int {
	return int(b.Sub(a).Seconds())
}

func computeDurationSecs(owner, repo, token string, deployment Deployment) (int, error) {
	successStatus, err := fetchSuccessStatus(owner, repo, token, deployment.ID)
	if err != nil {
		return 0, err
	}
	if successStatus != nil {
		return durationSeconds(deployment.CreatedAt, successStatus.CreatedAt), nil
	}
	return 0, nil
}

func computeStats(owner, repo, token string, deployments []Deployment) (Stats, error) {
	var durations []int
	var total, sum int

	for _, deployment := range deployments {
		duration, err := computeDurationSecs(owner, repo, token, deployment)
		if err != nil {
			return Stats{}, err
		}
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
			MinDurationSecs: min(durations),
			MaxDurationSecs: max(durations),
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

func run(owner, repo, environment, cutoffISO8601 string) error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return errors.New("GITHUB_TOKEN environment variable is not set")
	}

	deployments, err := fetchDeployments(owner, repo, environment, token, 1)
	if err != nil {
		return err
	}

	fmt.Printf("Fetched %d deployments for %s:\n", len(deployments), environment)

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

		oldStats, err := computeStats(owner, repo, token, oldDeployments)
		if err != nil {
			return err
		}
		newStats, err := computeStats(owner, repo, token, newDeployments)
		if err != nil {
			return err
		}

		printStats("old", oldStats)
		printStats("new", newStats)
	} else {
		stats, err := computeStats(owner, repo, token, deployments)
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

	if err := run(owner, repo, environment, cutoffISO8601); err != nil {
		log.Fatal(err)
	}
}

func min(arr []int) int {
	minVal := arr[0]
	for _, v := range arr {
		if v < minVal {
			minVal = v
		}
	}
	return minVal
}

func max(arr []int) int {
	maxVal := arr[0]
	for _, v := range arr {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}
