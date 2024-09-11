import {Octokit} from "@octokit/rest";
import {exec} from "child_process";
import util from "util";
import {getTime, isBefore, parseISO} from "date-fns";
import fetch from "node-fetch";
import meow from "meow";

const execAsync = util.promisify(exec);

const getGithubTokenFromCLI = async () => {
  try {
    const {stdout} = await execAsync("gh auth token");
    return stdout.trim();
  } catch (error) {
    console.error("Error fetching GitHub token from CLI:", error);
    process.exit(1);
  }
};

const initOctokit = async () => {
  const token = await getGithubTokenFromCLI();
  return new Octokit({
    auth: token,
    request: {fetch},
  });
};

const durationSeconds = (a, b) => Math.round((getTime(parseISO(b)) - getTime(parseISO(a))) / 1000);
const isBeforeCutoff = (cutoffISO8601) => {
  const cutoff = parseISO(cutoffISO8601);
  return (deployment) => isBefore(parseISO(deployment.created_at), cutoff);
};
const not = (fn) => (input) => !fn(input);

const computeDurationSecs = (octokit, owner, repo) => async (deployment) => {
  const successStatus = (
      await octokit.repos.listDeploymentStatuses({owner, repo, deployment_id: deployment.id})
  ).data.filter((status) => status.state === 'success')[0];

  if (successStatus === undefined) return null;

  return durationSeconds(deployment.created_at, successStatus.created_at);
};

const fetchDeploymentsFor = async (octokit, owner, repo, environment, totalPages) => {
  const deploymentss = await Promise.all(
      [...Array(totalPages).keys()].map((n) =>
          octokit.repos.listDeployments({owner, repo, environment, per_page: 100, page: n + 1}),
      ),
  );
  return deploymentss.flatMap((page) => page.data);
};

const computeStats = async (octokit, owner, repo, deployments) => {
  const durationsSecs = (await Promise.all(deployments.map(computeDurationSecs(octokit, owner, repo)))).filter(
      (analysis) => analysis !== null,
  );

  return {
    total: deployments.length,
    avgDurationSecs: Math.round(durationsSecs.reduce((a, b) => a + b, 0) / durationsSecs.length),
    minDurationSecs: Math.min(...durationsSecs),
    maxDurationSecs: Math.max(...durationsSecs),
  };
};

const printStats = (group, stats) => {
  console.log(
      `- ${stats.total} ${group} deployments: avg ${stats.avgDurationSecs} secs, min/max: ${stats.minDurationSecs}/${stats.maxDurationSecs} secs`,
  );
};

const run = async (owner, repo, environment, cutoffISO8601) => {
  const octokit = await initOctokit();

  const deployments = await fetchDeploymentsFor(octokit, owner, repo, environment, 5);
  console.log(`Fetched ${deployments.length} deployments for ${environment}:`);

  const deploymentBeforeCutoff = isBeforeCutoff(cutoffISO8601);
  printStats("old", await computeStats(octokit, owner, repo, deployments.filter(deploymentBeforeCutoff)));
  printStats("new", await computeStats(octokit, owner, repo, deployments.filter(not(deploymentBeforeCutoff))));
};

const cli = meow(`
  Usage
    $ gh deployment-stats --owner <owner> --repo <repo> --environment <environment> --cutoff <cutoffISO8601>
  
  Options
    --owner, -o       Repository owner (required)
    --repo, -r        Repository name (required)
    --environment, -e Environment name (required)
    --cutoff, -c      Cutoff date in ISO8601 format (required)
  
  Examples
    $ gh deployment-stats --owner foo --repo bar --environment prod --cutoff 2024-09-10T10:00:00Z
`, {
  importMeta: import.meta,
  flags: {
    owner: {
      type: 'string',
      shortFlag: 'o',
      isRequired: true
    },
    repo: {
      type: 'string',
      shortFlag: 'r',
      isRequired: true
    },
    environment: {
      type: 'string',
      shortFlag: 'e',
      isRequired: true
    },
    cutoff: {
      type: 'string',
      shortFlag: 'c',
      isRequired: true
    }
  }
});

try {
  await run(cli.flags.owner, cli.flags.repo, cli.flags.environment, cli.flags.cutoff);
} catch (e) {
  console.error(e);
}
