#!/bin/bash
# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may
# not use this file except in compliance with the License. A copy of the
# License is located at
#
#	http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is distributed
# on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# express or implied. See the License for the specific language governing
# permissions and limitations under the License.

# Script to generate changelog entries for commits since the last release.

set -e

# Default values
OUTPUT_FILE="CHANGELOG.md"
REPO_OWNER="aws"
REPO_NAME="amazon-ecs-agent"
BASE_BRANCH="master"
DEV_BRANCH="dev"
AGENT_VERSION=""

# Global arrays to store changelog entries by category
declare -a FEATURES
declare -a ENHANCEMENTS
declare -a BUGFIXES

# Usage function
usage() {
	echo "Usage: $0 --agent-version <X.Y.Z>"
	echo "Options:"
	echo " --agent-version    Agent version in X.Y.Z format (e.g., 1.102.0)"
	exit 1
}

# Cleanup function
TEMP_CHANGELOG_FILE=""
cleanup() {
	rm -f "$TEMP_CHANGELOG_FILE"
}

# Set trap to cleanup on exit
trap cleanup EXIT INT TERM

# Logs are redirected to stderr (>&2) to avoid interfering with functions that return data via stdout.
log_info() {
	echo "[INFO] $*" >&2
}

log_warn() {
	echo "[WARN] $*" >&2
}

log_error() {
	echo "[ERROR] $*" >&2
}

# Parse and validate version
parse_version() {
	local version="$1"

	# Check if version is provided
	if [ -z "$version" ]; then
		log_error "--agent-version is required"
		usage
		exit 1
	fi

	# Validate version format (X.Y.Z)
	if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
		log_error "Invalid version format. Expected X.Y.Z (e.g., 1.102.0)"
		exit 1
	fi

	# Set global agent version
	AGENT_VERSION="$version"
}

# Function to call GitHub API
github_api_call() {
	local endpoint="$1"
	local response http_code body

	local max_retries=5
	local backoff=2
	local retry_count=0

	while [ $retry_count -lt $max_retries ]; do
		# Ref: https://docs.github.com/en/rest/commits/commits?apiVersion=2022-11-28
		response=$(curl -s -w "\n%{http_code}" \
			-H "Accept: application/vnd.github+json" \
			-H "X-GitHub-Api-Version: 2022-11-28" \
			"https://api.github.com${endpoint}")

		http_code=$(echo "$response" | tail -n1)
		body=$(echo "$response" | sed '$d')

		# Success
		if [ "$http_code" -eq 200 ]; then
			echo "$body"
			return 0
		fi

		# Retry
		if [ "$http_code" -ge 500 ]; then
			retry_count=$((retry_count + 1))
			if [ $retry_count -lt $max_retries ]; then
				log_warn "GitHub API returned $http_code, retrying ($retry_count/$max_retries)..."
				sleep $backoff
				backoff=$((backoff * 2))
				continue
			fi
		fi

		# Fail
		log_error "GitHub API call failed for ${endpoint} - HTTP Code: ${http_code}"
		echo "$body" >&2
		return 1
	done
}

# Get PRs associated with a commit
get_prs_for_commit() {
	local commit_sha="$1"

	github_api_call "/repos/${REPO_OWNER}/${REPO_NAME}/commits/${commit_sha}/pulls"
}

# Get commits to include in the changelog
get_release_changes() {
	log_info "Getting commits eligible for the release..."
	local commits

	# Exclude merge commits using --no-merges
	commits=$(git log ${BASE_BRANCH}..${DEV_BRANCH} --no-merges --format=%H 2>/dev/null)

	if [ $? -ne 0 ]; then
		log_error "Failed to get commits. Make sure you're in a git repository and both ${BASE_BRANCH} and ${DEV_BRANCH} branches exist"
		exit 1
	fi

	if [ -z "$commits" ]; then
		log_warn "No commits to release (${DEV_BRANCH} is up to date with ${BASE_BRANCH})"
		exit 0
	fi

	echo "$commits"
}

# Extract changelog description from PR body
extract_changelog_description() {
	local pr_body="$1"

	if [ -z "$pr_body" ]; then
		return
	fi

	# Get the first non-empty, non-HTML-comment line after "Description for the changelog"
	# Ref: .github/PULL_REQUEST_TEMPLATE.md
	local changelog_desc
	changelog_desc=$(echo "$pr_body" | awk '
        /[Dd]escription for the [Cc]hangelog/ { flag=1; next }
        /^[[:space:]]*<!--/ { comment=1 }
        /-->/ { comment=0; next }
        comment { next }
        flag && NF > 0 { print; exit }
    ' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')

	if [ -z "$changelog_desc" ]; then
		return
	fi

	# Check if changelog should be skipped (N/A or Housekeeping)
	if echo "$changelog_desc" | grep -qiE '^(n/?a|none|not applicable|housekeeping)'; then
		return
	fi

	echo "$changelog_desc"
}

# Categorize changelog entry based on explicit category markers in description
# Returns: "Feature", "Bugfix", or "Enhancement"
categorize_entry() {
	local changelog_desc="$1"

	# Check for explicit category markers at the start of the description
	if echo "$changelog_desc" | grep -qiE '^(feature|feat)'; then
		echo "Feature"
	elif echo "$changelog_desc" | grep -qiE '^enhancement'; then
		echo "Enhancement"
	elif echo "$changelog_desc" | grep -qiE '^(bugfix|bug-?fix|bug|fix)'; then
		echo "Bugfix"
	else
		# Default to Enhancement if no category specified
		echo "Enhancement"
	fi
}

# Process PRs and extract changelog entries
process_prs() {
	local commits="$1"
	local commit_count
	commit_count=$(echo "$commits" | wc -l | tr -d ' ')

	log_info "Found ${commit_count} commits since last release"
	log_info "Processing commits and extracting changelog entries..."

	local processed_prs=()

	for commit_sha in $commits; do
		# Get PR associated with this commit
		local response
		response=$(get_prs_for_commit "$commit_sha")

		if [ $? -ne 0 ]; then
			log_error "Failed to fetch PRs for commit ${commit_sha}"
			exit 1
		fi

		# Filter only merged PRs, sort by PR number, and take the first PR
		response=$(echo "$response" | jq '[.[] | select(.merged_at != null)] | sort_by(.number) | .[0]')

		local pr_number
		pr_number=$(echo "$response" | jq -r ".number")

		# Check if no merged PR found for this commit
		if [ -z "$pr_number" ] || [ "$pr_number" = "null" ]; then
			log_warn "No merged PR found for commit ${commit_sha}, skipping"
			continue
		fi

		# Skip if we've already processed this PR
		if [[ " ${processed_prs[@]} " =~ " ${pr_number} " ]]; then
			log_info "Skipping already processed PR #${pr_number}"
			continue
		fi

		processed_prs+=("$pr_number")

		local pr_body pr_url pr_title
		pr_body=$(echo "$response" | jq -r ".body // empty")
		pr_url=$(echo "$response" | jq -r ".html_url")
		pr_title=$(echo "$response" | jq -r ".title // empty")

		# Extract changelog description, fall back to PR title if there's no changelog description
		local changelog_desc
		changelog_desc=$(extract_changelog_description "$pr_body")

		if [ -z "$changelog_desc" ]; then
			if [ -n "$pr_title" ]; then
				log_info "PR #${pr_number}: No changelog description, using PR title"
				changelog_desc="$pr_title"
			else
				log_warn "Skipping PR #${pr_number}: No changelog description or title"
				continue
			fi
		fi

		# Categorize the entry
		local category
		category=$(categorize_entry "$changelog_desc")

		# Format entry: "* Category - Description [#PR](URL)"
		local entry="* ${category} - ${changelog_desc} [#${pr_number}](${pr_url})"

		# Add to appropriate category array
		case "$category" in
		Feature)
			FEATURES+=("$entry")
			;;
		Bugfix)
			BUGFIXES+=("$entry")
			;;
		*)
			ENHANCEMENTS+=("$entry")
			;;
		esac
	done
}

# Update CHANGELOG.md file with new entries
update_changelog_file() {
	local agent_version="$1"

	# Check if we have any entries
	local total_entries=$((${#FEATURES[@]} + ${#ENHANCEMENTS[@]} + ${#BUGFIXES[@]}))
	if [ $total_entries -eq 0 ]; then
		log_warn "No changelog entries found in PRs"
	fi

	# Check if CHANGELOG.md exists
	if [ ! -f "$OUTPUT_FILE" ]; then
		log_error "${OUTPUT_FILE} not found"
		exit 1
	fi

	log_info "Adding new changelog entry for ${agent_version}..."

	# Create a temporary file with the new entry
	TEMP_CHANGELOG_FILE=$(mktemp)

	# Write the new entry to temp file (blank line, version, entries)
	echo "" >>"$TEMP_CHANGELOG_FILE"
	echo "# ${agent_version}" >>"$TEMP_CHANGELOG_FILE"

	# Add all entries (features first, then enhancements, then bugfixes)
	for entry in "${FEATURES[@]}"; do
		echo "${entry}" >>"$TEMP_CHANGELOG_FILE"
	done

	for entry in "${ENHANCEMENTS[@]}"; do
		echo "${entry}" >>"$TEMP_CHANGELOG_FILE"
	done

	for entry in "${BUGFIXES[@]}"; do
		echo "${entry}" >>"$TEMP_CHANGELOG_FILE"
	done

	# Insert the new entry after line 1 of CHANGELOG.md
	{
		head -n 1 "$OUTPUT_FILE"
		cat "$TEMP_CHANGELOG_FILE"
		tail -n +2 "$OUTPUT_FILE"
	} >"$OUTPUT_FILE.tmp" && mv "$OUTPUT_FILE.tmp" "$OUTPUT_FILE"

	log_info "Successfully updated ${OUTPUT_FILE}"
}

# Main function
main() {
	# Parse command line arguments
	local version=""

	while [[ $# -gt 0 ]]; do
		case $1 in
		--agent-version)
			version="$2"
			shift 2
			;;
		-h | --help)
			usage
			;;
		*)
			echo "Error: Unknown option $1"
			usage
			;;
		esac
	done

	# Parse version input
	parse_version "$version"

	# Check if changelog entry already exists
	if [ -f "$OUTPUT_FILE" ]; then
		if grep -q "^# ${AGENT_VERSION}$" "$OUTPUT_FILE"; then
			log_info "Changelog entry for version ${AGENT_VERSION} already exists"
			exit 0
		fi
	fi

	log_info "Generating changelog for version ${AGENT_VERSION}"

	# Get commits eligible for a release
	local commits
	commits=$(get_release_changes)

	# Process PRs and extract changelog entries
	process_prs "$commits"

	# Update CHANGELOG.md file
	update_changelog_file "$AGENT_VERSION"
}

# Run main function with all arguments
main "$@"
