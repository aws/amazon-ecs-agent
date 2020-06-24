package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

type Change struct {
	Version  string
	Name     string
	Datetime time.Time
	Changes  []string
}

const (
	CHANGE_TEMPLATE = "./CHANGELOG_MASTER"
	TOP_LEVEL       = "../../CHANGELOG.md"
	AMAZON_LINUX    = "../../packaging/amazon-linux-ami/ecs-init.spec"
	SUSE            = "../../packaging/suse/amazon-ecs-init.changes"
	UBUNTU          = "../../packaging/ubuntu-trusty/debian/changelog"

	AMAZON_LINUX_TIME_FMT = "Mon Jan 02 2006"
	DEBIAN_TIME_FMT       = "Mon, 02 Jan 2006 15:04:05 -0700"
	SUSE_TIME_FMT         = "Mon Jan 02, 15:04:05 MST 2006"
)

func main() {
	templateFile, err := os.Open(CHANGE_TEMPLATE)
	handleErr(err, "unable to open CHANGE_TEMPLATE")
	defer templateFile.Close()

	// parse CHANGE_TEMPLATE
	scanner := bufio.NewScanner(templateFile)
	changeArray := []Change{}
	changeStrings := []string{}
	for scanner.Scan() {
		thisText := scanner.Text()
		if thisText == "" {
			currentChanges := []string{}
			// parse the remaining array of changes
			for i := 3; i < len(changeStrings); i++ {
				currentChanges = append(currentChanges, changeStrings[i])
			}
			thisTime, err := time.Parse(time.RFC3339, changeStrings[2])
			handleErr(err, "error parsing time")
			thisChange := Change{
				Version:  changeStrings[0],
				Name:     changeStrings[1],
				Datetime: thisTime,
				Changes:  currentChanges,
			}
			changeArray = append(changeArray, thisChange)
			changeStrings = []string{}
		} else {
			changeStrings = append(changeStrings, thisText)
		}
	}

	if !validateChangelog(changeArray) {
		fmt.Println("Master Changelog is invalid.")
		return
	}

	// Create formatted strings for each log
	amazonLinuxChangeString := getAmazonLinuxChangeString(changeArray)
	ubuntuChangeString := getUbuntuChangeString(changeArray)
	suseChangeString := getSuseChangeString(changeArray)
	topLevelChangeString := getTopLevelChangeString(changeArray)

	// update changelog files
	rewriteChangelog(TOP_LEVEL, topLevelChangeString)
	rewriteChangelog(UBUNTU, ubuntuChangeString)
	rewriteChangelog(SUSE, suseChangeString)

	// find changelog in AmazonLinux rpm spec
	var amazonLinuxSpecBase = ""
	if _, err := os.Stat(AMAZON_LINUX); err == nil {
		f, err := os.Open(AMAZON_LINUX)
		handleErr(err, "unable to open amazon linux changelog")
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "%changelog") {
				amazonLinuxSpecBase += "%changelog\n"
				break
			}
			amazonLinuxSpecBase += fmt.Sprintln(line)
		}
	}
	completeAmazonLinuxChangeString := amazonLinuxSpecBase + amazonLinuxChangeString
	rewriteChangelog(AMAZON_LINUX, completeAmazonLinuxChangeString)
}

func validateChangelog(allChange []Change) bool {
	//validate that all dates are in ascending order
	for i := 1; i < len(allChange); i++ {
		prevTime := allChange[i].Datetime
		nextTime := allChange[i-1].Datetime
		if !prevTime.Before(nextTime) {
			fmt.Printf("datetimes out of order: %s should be after %s\n", prevTime.String(), nextTime.String())
			return false
		}
	}
	//validate that there are no duplicate versions
	versionSet := make(map[string]bool)
	for _, change := range allChange {
		versionSet[change.Version] = true
	}
	if len(versionSet) != len(allChange) {
		fmt.Printf("there's a duplicate version present\n")
		return false
	}
	return true
}

// format as follows:
//
// * Wed Jan 08 2020 Cameron Sparr <cssparr@amazon.com> - 1.36.0-1
// - Cache Agent version 1.36.0
// - Capture a fixed tail of container logs when removing a container
func getAmazonLinuxChangeString(allChange []Change) string {
	result := ""
	for _, change := range allChange {
		thisTime := change.Datetime.UTC().Format(AMAZON_LINUX_TIME_FMT)
		result += fmt.Sprintf("* %s %s - %s\n", thisTime, change.Name, change.Version)
		for _, update := range change.Changes {
			result += fmt.Sprintf("- %s\n", update)
		}
		result += fmt.Sprintln()
	}
	return result
}

// format as follows:
//
// amazon-ecs-init (1.36.0-1) trusty; urgency=medium
//
//  * Cache Agent version 1.36.0
//  * capture a fixed tail of container logs when removing a container
//
//  -- Cameron Sparr <cssparr@amazon.com> Wed, 08 Jan 2020 11:00:00 -0800
func getUbuntuChangeString(allChange []Change) string {
	result := ""
	for _, change := range allChange {
		result += fmt.Sprintf("amazon-ecs-init (%s) trusty; urgency=medium\n\n", change.Version)
		thisTime := change.Datetime.UTC().Format(DEBIAN_TIME_FMT)
		for _, update := range change.Changes {
			result += fmt.Sprintf("  * %s\n", update)
		}
		result += fmt.Sprintln()
		result += fmt.Sprintf(" -- %s  %s\n", change.Name, thisTime)
		result += fmt.Sprintln()
	}
	return result
}

// format as follows
// -------------------------------------------------------------------
// Tue Apr 22 20:54:26 UTC 2013 - your@email.com
//
// - level 1 bullet point; long descriptions
//   should wrap
// - another l1 bullet point
func getSuseChangeString(allChange []Change) string {
	result := ""
	for _, change := range allChange {
		result += fmt.Sprintf("-------------------------------------------------------------------\n")
		thisTime := change.Datetime.UTC().Format(SUSE_TIME_FMT)
		thisEmail := getEmailSlice(change.Name)
		result += fmt.Sprintf("%s - %s - %s\n\n", thisTime, thisEmail, change.Version)
		for _, update := range change.Changes {
			result += fmt.Sprintf("- %s\n", update)
		}
	}
	return result
}

// format as follows
//
//  ## 1.35.0
//  * Cache Agent version 1.36.0
//  * capture a fixed tail of container logs when removing a container
func getTopLevelChangeString(allChange []Change) string {
	result := "# Changelog\n\n"
	for _, change := range allChange {
		result += fmt.Sprintf("## %s\n", change.Version)
		for _, update := range change.Changes {
			result += fmt.Sprintf("* %s\n", update)
		}
		result += fmt.Sprintln()
	}
	// escape special char for .md
	r := strings.NewReplacer("_", "\\_")
	result = r.Replace(result)
	return result
}

// update changelog files
// removes original file, creates updated file
func rewriteChangelog(changelogFile string, updateString string) {
	_, err := os.Stat(changelogFile)
	if err != nil {
		handleErr(err, "unable to stat changelog file: "+changelogFile)
	} else {
		err := os.Remove(changelogFile)
		handleErr(err, "unable to remove changelog file: "+changelogFile)
		f, err := os.Create(changelogFile)
		handleErr(err, "unable to create changelog file: "+changelogFile)
		defer f.Close()
		_, err = f.WriteString(updateString)
		handleErr(err, "unable to write string to changelog file: "+changelogFile)
	}
}

// Simple error print.  Passthough if err is nil
func handleErr(err error, descriptor string) {
	if err != nil {
		fmt.Println(descriptor, err)
		return
	}
}

func getEmailSlice(fullName string) string {
	startIndex := strings.Index(fullName, "<")
	endIndex := strings.Index(fullName, ">")
	runes := []rune(fullName)
	return string(runes[startIndex+1 : endIndex])
}
