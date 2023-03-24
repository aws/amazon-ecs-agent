#!/bin/bash

GITHUB_DEST_BRANCH_NAME=$(echo ${CODEBUILD_WEBHOOK_BASE_REF} | cut -d "/" -f3-)
GIT_COMMIT_SHA=$(git rev-parse $GITHUB_DEST_BRANCH_NAME)
GIT_COMMIT_SHORT_SHA=$(git rev-parse --short=8 $GITHUB_DEST_BRANCH_NAME)
RELEASE_DATE=$(date +'%Y%m%d')
AGENT_VERSION=$(cat ../VERSION)

cat << EOF > agent.json
{
    "agentReleaseVersion" : "$AGENT_VERSION",
    "releaseDate" : "$RELEASE_DATE",
    "initStagingConfig": {
    "release": "1"
    },
    "agentStagingConfig": {
    "releaseGitSha": "$GIT_COMMIT_SHA",
    "releaseGitShortSha": "$GIT_COMMIT_SHORT_SHA",
    "gitFarmRepoName": "MadisonContainerAgentExternal",
    "gitHubRepoName": "aws/amazon-ecs-agent",
    "gitFarmStageBranch": "v${AGENT_VERSION}-stage",
    "githubReleaseUrl": ""
    }
}
EOF

# Downloading the agentVersionV2-<branch>.json (it is assumed that it already exists)
aws s3 cp ${RESULTS_BUCKET_URI}/agentVersionV2/agentVersionV2-${GITHUB_SOURCE_BRANCH_NAME}.json ./agentVersionV2-${GITHUB_SOURCE_BRANCH_NAME}.json
echo "Downloaded agentVersionV2-${GITHUB_SOURCE_BRANCH_NAME}.json"
cat agentVersionV2-${GITHUB_SOURCE_BRANCH_NAME}.json

# Grabbing the new current and release agent version
CURR_VER=$(jq -r '.agentReleaseVersion' agentVersionV2-${GITHUB_SOURCE_BRANCH_NAME}.json)

# Checking to see if there's a new release agent version to be released
if [[ ! $AGENT_VERSION =~ "${CURR_VER}" ]] ; then
    # Updating the agentVersionV2-<branch>.json file with new current and release agent versions and copying it to a temp file
    cat <<< $(jq '.agentReleaseVersion = '\"$AGENT_VERSION\"' | .agentCurrentVersion = '\"$CURR_VER\"'' agentVersionV2-${GITHUB_SOURCE_BRANCH_NAME}.json) > agentVersionV2-${GITHUB_SOURCE_BRANCH_NAME}-COPY.json
    # Replace existing agentVersionV2-<branch>.json file with the temp file
    jq . agentVersionV2-${GITHUB_SOURCE_BRANCH_NAME}-COPY.json > agentVersionV2-${GITHUB_SOURCE_BRANCH_NAME}.json
fi

# Uploading new agentVersionV2-<branch>.json and agent.json files
echo "Uploading agentVersionV2-${GITHUB_SOURCE_BRANCH_NAME}.json"
cat agentVersionV2-${GITHUB_SOURCE_BRANCH_NAME}.json
aws s3 cp ./agentVersionV2-${GITHUB_SOURCE_BRANCH_NAME}.json ${RESULTS_BUCKET_URI}/agentVersionV2/agentVersionV2-${GITHUB_SOURCE_BRANCH_NAME}.json

echo "Uploading agent.json"
cat agent.json
aws s3 cp ./agent.json ${RESULTS_BUCKET_URI}/${GITHUB_SOURCE_BRANCH_NAME}/agent.json