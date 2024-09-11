import https from "https";

export default (owner, repo, token) => {
  const request = async (path) => {
    return new Promise((resolve, reject) => {
      const options = {
        hostname: 'api.github.com',
        path,
        method: 'GET',
        headers: {
          'User-Agent': 'Node.js',
          'Authorization': `token ${token}`,
          'Accept': 'application/vnd.github.v3+json'
        }
      };

      const req = https.request(options, (res) => {
        let data = '';

        res.on('data', (chunk) => {
          data += chunk
        });
        res.on('end', () => {
          try {
            resolve(JSON.parse(data));
          } catch (err) {
            reject(`Error parsing response: ${err.message}`);
          }
        });
      });

      req.on('error', (err) => {
        reject(`Request error: ${err.message}`)
      });
      req.end();
    });
  }

  const fetchDeployments = async (environment, totalPages) => {
    const deployments = await Promise.all(
        Array.from({length: totalPages}, (_, i) =>
            request(`/repos/${owner}/${repo}/deployments?environment=${environment}&per_page=100&page=${i + 1}`)
        )
    );
    return deployments.flatMap((i) => i);
  }

  const fetchSuccessStatus = async (deploymentId) => {
    const statuses = await request(`/repos/${owner}/${repo}/deployments/${deploymentId}/statuses`)
    return statuses.find((status) => status.state === "success");
  }

  return {fetchDeployments, fetchSuccessStatus}
}