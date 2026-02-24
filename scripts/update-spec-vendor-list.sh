#!/bin/bash
# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License").
# You may not use this file except in compliance with the License.
# A copy of the License is located at
#
#     http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is
# distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
# CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and
# limitations under the License.

# This script updates the bundled Go dependencies list in the RPM spec files
# based on the current vendored dependencies in ecs-init

set -e

# Cleanup temporary files on exit
cleanup() {
    rm -f "$TEMP_PROVIDES" "$TEMP_SPEC" 2>/dev/null
}
trap cleanup EXIT

# Normalize to working directory being build root
ROOT=$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )
cd "${ROOT}"

SPEC_FILES=(
    "packaging/amazon-linux-ami-integrated/ecs-agent.spec"
)

echo "Generating bundled dependencies list from ecs-init/vendor..."

# Generate the new Provides list based off the ecs-init/vendor directory
NEW_PROVIDES=$(find ecs-init/vendor -name \*.go -exec dirname {} \; | \
    sort | uniq | \
    sed 's,^.*ecs-init/vendor/,,; s/^/bundled(golang(/; s/$/))/;' | \
    sed 's/^/Provides:\t/' | \
    expand)

PROVIDES_COUNT=$(echo "$NEW_PROVIDES" | wc -l | tr -d ' ')

printf "Found %s bundled packages\n\n" "$PROVIDES_COUNT"

UPDATED_COUNT=0

# Update each spec file
for SPEC_FILE in "${SPEC_FILES[@]}"; do
    if [ ! -f "$SPEC_FILE" ]; then
        echo "Warning: $SPEC_FILE not found, skipping..."
        continue
    fi

    echo "Updating $SPEC_FILE..."

    # Create a temporary file with the new provides list
    TEMP_PROVIDES=$(mktemp)
    echo "$NEW_PROVIDES" > "$TEMP_PROVIDES"

    # Create a temporary file for the output
    TEMP_SPEC=$(mktemp)

    # Use awk to replace the Provides section
    # Strategy: Find first and last Provides line with bundled(golang(, replace everything between
    awk -v provides_file="$TEMP_PROVIDES" '
    BEGIN { 
        in_provides=0
        provides_printed=0
    }
    # Detect start of bundled golang Provides section
    /^Provides:.*bundled\(golang\(/ && !in_provides { 
        if (!provides_printed) {
            # Read and print the new provides list from file
            while ((getline line < provides_file) > 0) {
                print line
            }
            close(provides_file)
            provides_printed=1
        }
        in_provides=1
        next
    }
    # Skip all Provides lines in the section
    in_provides && /^Provides:.*bundled\(golang\(/ {
        next
    }
    # End of Provides section (any line that is not a bundled golang Provides)
    in_provides && !/^Provides:.*bundled\(golang\(/ {
        in_provides=0
    }
    # Print all other lines
    !in_provides { print }
    ' "$SPEC_FILE" > "$TEMP_SPEC"

    # Check if file actually changed
    if ! cmp -s "$SPEC_FILE" "$TEMP_SPEC"; then
        # Replace the original file
        mv "$TEMP_SPEC" "$SPEC_FILE"
        echo "  ✓ Updated $SPEC_FILE"
        UPDATED_COUNT=$((UPDATED_COUNT + 1))
    else
        echo "  - No changes needed for $SPEC_FILE"
    fi
done

echo ""
if [ $UPDATED_COUNT -gt 0 ]; then
    echo "Summary: Updated $UPDATED_COUNT spec file(s) with $PROVIDES_COUNT bundled packages"
else
    echo "No changes needed - vendored dependencies list is already up-to-date"
fi
