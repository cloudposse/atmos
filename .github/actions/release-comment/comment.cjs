// Preserve the existing release notification behavior.
module.exports = async ({ github, context }) => {
  // Function to check if a value is unique in an array
  function onlyUnique(value, index, array) {
    return array.indexOf(value) === index;
  }

  // Function to create or update a comment for a pull request (PR) associated with a release
  async function createCommentForPR(pr_id, release) {
    // Constructing the message to be posted or updated as a comment
    const messageId = `<!-- release-pr-comment:${release.id} -->`;
    const message = `
    ${messageId}
    These changes were released in [${release.name}](${release.html_url}).
    `;

    // Сreate a new comment
    await github.rest.issues.createComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      issue_number: pr_id,
      body: message
    });
  }

  // Retrieving the ID of the current release
  const release_id = context.payload.release.id;

  // Fetching details of the current release
  const currentReleaseResponse = await github.rest.repos.getRelease({
    owner: context.repo.owner,
    repo: context.repo.repo,
    release_id,
  });

  const currentRelease = currentReleaseResponse.data;

  // Extracting tag name and target branch from the current release
  const currentTag = currentRelease.tag_name;

  // Listing all releases of the repository
  const releases = await github.rest.repos.listReleases({
    owner: context.repo.owner,
    repo: context.repo.repo,
  });

  // Initializing variables for storing information about the previous release
  let previousRelease = null;
  let currentReleaseFound = false;

  // Iterating through releases to find the previous release relative to the current one
  for (const release of releases.data) {
    if (currentReleaseFound) {
      previousRelease = release;
      break;
    } else if (release.tag_name == currentTag) {
      currentReleaseFound = true;
    }
  }

  // If no previous release is found, log a message and return
  if (previousRelease == null) {
    console.log(`No previous release found for ${currentTag}`);
    return;
  }

  // Comparing commits between the current and previous releases
  const commitsResponse = await github.rest.repos.compareCommits({
    owner: context.repo.owner,
    repo: context.repo.repo,
    base: previousRelease.tag_name,
    head: currentRelease.tag_name,
  });

  const commits = commitsResponse.data;

  // Initializing an array to store pull request numbers associated with the commits
  const pull_requests = [];

  // Iterating through commits to find associated pull requests and extracting their numbers
  for (const commit of commits.commits) {
    // GraphQL query to fetch details about the commit, including associated pull requests
    const query = `
      {
        resource(url: "${context.payload.repository.html_url}/commit/${commit.sha}") {
          ... on Commit {
            messageHeadlineHTML
            messageBodyHTML
            associatedPullRequests(first: 10) {
              pageInfo { hasNextPage }
              edges { node { number } }
            }
          }
        }
      }
    `;

    const response = await github.graphql(query);

    // Extracting pull request numbers from the GraphQL response
    for (const edge of response.resource.associatedPullRequests.edges) {
      pull_requests.push(edge.node.number);
    }
  }

  // Iterating through unique pull request numbers and creating or updating comments for them
  for (const id of pull_requests.filter(onlyUnique)) {
    await createCommentForPR(id, currentRelease);
  }

};
