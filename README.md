# GitHub Deployment Stats

The GitHub Deployment Stats CLI extension extracts run time stats about your GitHub Deployments.

The extension fetches information about your GitHub Deployments and their statuses to calculate the time it took for them to successfully complete. Then it computes the average, maximum and minimum run times.

Optionally, the extension can use a cutoff date (when provided) to separate the stats into two groups (before, and after the cutoff date).

## Getting started

Install the extension via this command:

```bash
gh extension install ggalmazor/gh-deployment-stats
```

Fetch some stats for one of your repos:

```bash
gh deployment-stats foo bar main
Fetched 500 deployments for main:
... fetching deployment statuses .................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................... Done
- 483 successful deployments: avg 196 secs, min/max: 55/684 secs
```

Set the maximum number of deployments to fetch with the `-deployments` flag:

```bash
gh deployment-stats -deployments 10 foo bar main
Fetched 10 deployments for main:
... fetching deployment statuses .......... Done
- 10 successful deployments: avg 171 secs, min/max: 55/321 secs
```

Provide a cutoff datetime in ISO8601 format to segregate results into two groups:

```bash
gh deployment-stats -cutoff 2024-08-01T12:00:00Z foo bar main
Fetched 500 deployments for main:
... fetching deployment statuses .................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................... Done
- 182 old successful deployments: avg 181 secs, min/max: 127/664 secs
- 301 new successful deployments: avg 206 secs, min/max: 55/684 secs
```

### Usage

```bash
gh deployment-stats -h
Usage: gh-deployment-stats [-deployments] [-cutoff] <owner> <repo> <environment>

Positional Arguments:
  owner         GitHub repository owner (required)
  repo          GitHub repository name (required)
  environment   Deployment environment (required)

Options:
  -cutoff string
    	Cutoff timestamp in ISO8601 format to divide results in two groups (optional)
  -deployments int
    	Total number of deployments to consider (optional) (default 500)
```