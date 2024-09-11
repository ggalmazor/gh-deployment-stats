import {Octokit} from "@octokit/rest";
import {exec} from "child_process";
import fetch from "node-fetch";

const getGithubTokenFromCLI = () =>
    new Promise((resolve, _) => {
      exec("gh auth token", (error, stdout) => {
        if (error) {
          console.error("Error fetching GitHub token from CLI:", error);
          process.exit(1);
        }
        resolve(stdout.trim());
      });
    });

const initOctokit = async () => {
  const token = await getGithubTokenFromCLI();
  return new Octokit({
    auth: token,
    request: {fetch},
  });
};

const durationSeconds = (a, b) => Math.round((new Date(b) - new Date(a)) / 1000);

const computeDurationSecs = (octokit, owner, repo) => async (deployment) => {
  const statuses = await octokit.repos.listDeploymentStatuses({
    owner,
    repo,
    deployment_id: deployment.id,
  });
  const successStatus = statuses.data.find((status) => status.state === "success");
  return successStatus ? durationSeconds(deployment.created_at, successStatus.created_at) : null;
};

const fetchDeploymentsFor = async (octokit, owner, repo, environment, totalPages) => {
  const deployments = await Promise.all(
      Array.from({length: totalPages}, (_, i) =>
          octokit.repos.listDeployments({
            owner,
            repo,
            environment,
            per_page: 100,
            page: i + 1,
          })
      )
  );
  return deployments.flatMap((page) => page.data);
};

const computeStats = async (octokit, owner, repo, deployments) => {
  const durations = (await Promise.all(deployments.map(computeDurationSecs(octokit, owner, repo)))).filter(Boolean);
  const total = durations.length;
  const sum = durations.reduce((a, b) => a + b, 0);
  return {
    total,
    avgDurationSecs: Math.round(sum / total),
    minDurationSecs: Math.min(...durations),
    maxDurationSecs: Math.max(...durations),
  };
};

const printStats = (group, stats) => {
  let groupMessage = group === null ? '' : `${group} `;
  console.log(
      `- ${stats.total} ${groupMessage}successful deployments: avg ${stats.avgDurationSecs} secs, min/max: ${stats.minDurationSecs}/${stats.maxDurationSecs} secs`
  );
};

const run = async (owner, repo, environment, cutoffISO8601) => {
  const octokit = await initOctokit();

  const deployments = await fetchDeploymentsFor(octokit, owner, repo, environment, 5);
  console.log(`Fetched ${deployments.length} deployments for ${environment}:`);

  if (cutoffISO8601) {
    const cutoff = new Date(cutoffISO8601);
    const oldDeployments = deployments.filter((d) => new Date(d.created_at) < cutoff);
    const newDeployments = deployments.filter((d) => new Date(d.created_at) >= cutoff);

    printStats("old", await computeStats(octokit, owner, repo, oldDeployments));
    printStats("new", await computeStats(octokit, owner, repo, newDeployments));
  } else {
    printStats(null, await computeStats(octokit, owner, repo, deployments));
  }
};

const [owner, repo, environment, cutoffISO8601] = process.argv.slice(2);

if (!owner || !repo || !environment) {
  console.error("Usage: <owner> <repo> <environment> [cutoffISO8601]");
  process.exit(1);
}

run(owner, repo, environment, cutoffISO8601).catch(console.error);