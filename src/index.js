import {exec} from "child_process";
import GHApiHelper from "./gh-api-helper.js";

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

const durationSeconds = (a, b) => Math.round((new Date(b) - new Date(a)) / 1000);

const computeDurationSecs = (ghApiHelper) => async (deployment) => {
  const successStatus = await ghApiHelper.fetchSuccessStatus(deployment.id)
  return successStatus ? durationSeconds(deployment.created_at, successStatus.created_at) : null;
};

const computeStats = async (ghApiHelper, deployments) => {
  const durations = (await Promise.all(deployments.map(computeDurationSecs(ghApiHelper)))).filter(Boolean);
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
  const token = await getGithubTokenFromCLI();
  const ghApiHelper = GHApiHelper(owner, repo, token);

  const deployments = await ghApiHelper.fetchDeployments(environment, 5);
  console.log(`Fetched ${deployments.length} deployments for ${environment}:`);

  if (cutoffISO8601) {
    const cutoff = new Date(cutoffISO8601);
    const oldDeployments = deployments.filter((d) => new Date(d.created_at) < cutoff);
    const newDeployments = deployments.filter((d) => new Date(d.created_at) >= cutoff);

    printStats("old", await computeStats(ghApiHelper, oldDeployments));
    printStats("new", await computeStats(ghApiHelper, newDeployments));
  } else {
    printStats(null, await computeStats(ghApiHelper, deployments));
  }

  process.exit(0);
};

const [owner, repo, environment, cutoffISO8601] = process.argv.slice(2);

if (!owner || !repo || !environment) {
  console.error("Usage: <owner> <repo> <environment> [cutoffISO8601]");
  process.exit(1);
}

run(owner, repo, environment, cutoffISO8601).catch(error => {
  console.error(error);
  process.exit(1);
});