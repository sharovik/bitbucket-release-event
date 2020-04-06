package bitbucket_release

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sharovik/devbot/internal/container"
	"github.com/sharovik/devbot/internal/dto"
	"github.com/sharovik/devbot/internal/log"
)

func failedPullRequestsText(failedPullRequests map[string]failedToMerge) string {
	if len(failedPullRequests) == 0 {
		return "All pull-requests are ready for merge! This is awesome!"
	}

	var text = "These pull-requests cannot be merged:\n"

	for pullRequest, reason := range failedPullRequests {
		text += fmt.Sprintf("%s - %s \n", pullRequest, reason.Reason)
	}

	return text
}

func canBeMergedPullRequestsText(canBeMerged map[string]PullRequest) string {
	if len(canBeMerged) == 0 {
		return "There is no pull-requests, which can be merged."
	}

	var text = "Next pull-requests is will be merged:\n"

	for pullRequestURL, pullRequest := range canBeMerged {
		text += fmt.Sprintf("[#%d] %s \n", pullRequest.ID, pullRequestURL)
	}

	return text
}

func checkPullRequests(items []PullRequest) (map[string]PullRequest, map[string]map[string]PullRequest, map[string]failedToMerge) {
	var (
		failedPullRequests         = make(map[string]failedToMerge)
		canBeMergedPullRequestList = make(map[string]PullRequest)
		canBeMergedByRepository    = make(map[string]map[string]PullRequest)
	)
	for _, pullRequest := range items {
		cleanPullRequestURL := fmt.Sprintf("https://bitbucket.org/%s/%s/pull-requests/%d", pullRequest.Workspace, pullRequest.RepositorySlug, pullRequest.ID)
		info, err := container.C.BibBucketClient.PullRequestInfo(pullRequest.Workspace, pullRequest.RepositorySlug, pullRequest.ID)
		if err != nil {
			failedPullRequests[cleanPullRequestURL] = failedToMerge{
				Reason: err.Error(),
				Info:   info,
				Error:  err,
			}

			continue
		}

		replacer := strings.NewReplacer("\\", "")
		pullRequest.Title = info.Title
		pullRequest.Description = replacer.Replace(info.Description)

		if !isPullRequestAlreadyMerged(info) {
			failedPullRequests[cleanPullRequestURL] = failedToMerge{
				Reason: fmt.Sprintf("The state should be %s, instead of it %s received.", pullRequestStateOpen, info.State),
				Info:   info,
				Error:  nil,
			}

			continue
		}

		isRequiredReviewersExistsInPullRequest, reason := checkIfRequiredReviewersExists(info)
		if !isRequiredReviewersExistsInPullRequest {
			failedPullRequests[cleanPullRequestURL] = reason
			continue
		}

		isPullRequestApprovedByReviewers := checkIfOneOfRequiredReviewersApprovedPullRequest(info)
		if !isPullRequestApprovedByReviewers {
			failedPullRequests[cleanPullRequestURL] = failedToMerge{
				Reason: "The pull-request should be approved by one of the required reviewers.",
				Info:   info,
				Error:  nil,
			}

			continue
		}

		log.Logger().Debug().
			Interface("pull_request", pullRequest).
			Msg("The pull-request can be merged.")
		canBeMergedPullRequestList[cleanPullRequestURL] = pullRequest

		if canBeMergedByRepository[pullRequest.RepositorySlug] == nil {
			canBeMergedByRepository[pullRequest.RepositorySlug] = make(map[string]PullRequest)
		}

		canBeMergedByRepository[pullRequest.RepositorySlug][pullRequest.Title] = pullRequest
	}

	return canBeMergedPullRequestList, canBeMergedByRepository, failedPullRequests
}

func releaseThePullRequests(canBeMergedPullRequestList map[string]PullRequest, canBeMergedByRepository map[string]map[string]PullRequest) (string, error) {
	log.Logger().StartMessage("Merge of received pull-requests")

	releaseText := ""
	//In case when we have only one pull-request we will merge it straight to the main branch
	if len(canBeMergedPullRequestList) == 1 {
		log.Logger().Debug().Msg("There is only 1 received pull-request. Trying to merge it.")
		releaseText = fmt.Sprintf("We have only one pull-request, so I will try to merge it directly to the main branch.\n")
		newText, err := mergePullRequests(canBeMergedPullRequestList)
		releaseText += fmt.Sprintf("%s\n", newText)
		if err != nil {
			log.Logger().AddError(err).Msg("Failed to merge the pull-request")
			log.Logger().FinishMessage("Merge of received pull-requests")
			return releaseText, err
		}

		log.Logger().FinishMessage("Merge of received pull-requests")
		return releaseText, nil
	}

	//Here we take sorted by repository pull-requests and trying to merge them into main or release branch.
	//We go in for loop into each repository and check how many pull-requests do we have there.
	//If only one, then we merge it into main branch, otherwise we create release branch for selected repository,
	//switch direction of the pull-requests to that release branch and merge all of them.
	for repository, pullRequests := range canBeMergedByRepository {
		//Well, in that case we have only one pull-request so we merge it into main branch
		if len(pullRequests) == 1 {
			log.Logger().Debug().Str("repository", repository).Msg("Only one pull-request received for selected repository")

			releaseText = fmt.Sprintf("There is only one pull-request for selected repository `%s`.", repository)
			newText, err := mergePullRequests(pullRequests)
			releaseText += fmt.Sprintf("%s\n", newText)
			if err != nil {
				log.Logger().AddError(err).Msg("Received error during pull-request merge")
			}

			continue
		}

		//This is for multiple pull-requests links
		var (
			repositories                  = map[string]dto.BitBucketResponseBranchCreate{}
			workspace                     = ""
			releasePullRequestDescription = ""
			releaseBranchName             = fmt.Sprintf("release/%s", time.Now().Format("2006.01.02"))
		)

		//In that case we have multiple pull-requests for that repository, so we have to create a release branch
		for _, pullRequest := range pullRequests {
			if workspace == "" {
				workspace = pullRequest.Workspace
			}

			releasePullRequestDescription += fmt.Sprintf("%s\n", pullRequest.Description)

			//If we don't have any created release branch for this repository the we need to create it
			if repositories[repository].Name == "" {
				branchResponse, err := container.C.BibBucketClient.CreateBranch(pullRequest.Workspace, pullRequest.RepositorySlug, releaseBranchName)
				if err != nil {
					releaseText += fmt.Sprintf("\nThe release-branch for repository %s cannot be created, because of `%s`", repository, err)
					log.Logger().AddError(err).Msg("Received an error during the release branch creation")
					return releaseText, err
				}

				repositories[repository] = branchResponse
			}

			//We switch the destination of the pull-request to the release branch
			_, err := container.C.BibBucketClient.ChangePullRequestDestination(
				pullRequest.Workspace,
				pullRequest.RepositorySlug,
				pullRequest.ID,
				prepareReleaseTitle(pullRequest.Title),
				releaseBranchName)
			if err != nil {
				releaseText += fmt.Sprintf("I've tried to switch the destination for pull-request #%d and I failed. Reason: `%s`", pullRequest.ID, err)
				log.Logger().AddError(err).Msg("Received an error during the branch destination switch")
				break
			}
		}

		releaseText += fmt.Sprintf("\nTrying to merge the %d pull-requests to the `%s` branch  of `%s` repository", len(pullRequests), releaseBranchName, repository)
		newText, err := mergePullRequests(pullRequests)
		releaseText += fmt.Sprintf("\n%s", newText)
		if err != nil {
			log.Logger().AddError(err).Msg("Received error during pull-request merge")
			log.Logger().FinishMessage("Merge of received pull-requests")
			return releaseText, err
		}

		//Now we need to create the pull-request
		pullRequestLink, err := createReleasePullRequest(workspace, repository, repositories[repository], releasePullRequestDescription)
		if err != nil {
			releaseText += fmt.Sprintf("\nI tried to create the release pull-request and I failed. Reason: %s", err)
			return releaseText, err
		}

		releaseText += fmt.Sprintf("\nPlease approve release pull-request: %s", pullRequestLink)
	}

	log.Logger().FinishMessage("Merge of received pull-requests")
	return releaseText, nil
}

func createReleasePullRequest(workspace string, repository string, releaseBranch dto.BitBucketResponseBranchCreate, description string) (string, error) {
	bitBucketPullRequestCreate := dto.BitBucketRequestPullRequestCreate{
		Title:       "Release pull-request",
		Description: description,
		Source: dto.BitBucketPullRequestDestination{
			Branch: dto.BitBucketPullRequestDestinationBranch{
				Name: releaseBranch.Name,
			},
		},
	}

	//Append reviewers to this pull-request except the author of the branch(which is current user)
	for _, reviewerUuID := range container.C.Config.BitBucketConfig.RequiredReviewers {
		if reviewerUuID.UUID != container.C.Config.BitBucketConfig.CurrentUserUUID {
			bitBucketPullRequestCreate.Reviewers = append(bitBucketPullRequestCreate.Reviewers, dto.BitBucketReviewer{UUID: reviewerUuID.UUID})
		}
	}

	fmt.Println(bitBucketPullRequestCreate.Reviewers)

	response, err := container.C.BibBucketClient.CreatePullRequest(workspace, repository, bitBucketPullRequestCreate)
	if err != nil {
		return "", err
	}

	if response.Links.HTML.Href == "" {
		log.Logger().Warn().Interface("response", response).Msg("There is no pull-request link in response.")
		return "", errors.New("The pull-request link was not found in the response. ")
	}

	return response.Links.HTML.Href, nil
}

func mergePullRequests(pullRequests map[string]PullRequest) (string, error) {
	var (
		releaseText     string
		repository      = ""
		lastPullRequest = PullRequest{}
	)

	for _, pullRequest := range pullRequests {
		lastPullRequest = pullRequest

		if repository == "" {
			repository = pullRequest.RepositorySlug
		}

		response, err := container.C.BibBucketClient.MergePullRequest(pullRequest.Workspace, pullRequest.RepositorySlug, pullRequest.ID, pullRequest.Description)
		if err != nil {
			releaseText += fmt.Sprintf("I cannot merge the pull-request #%d because of error `%s`", pullRequest.ID, err.Error())
			log.Logger().Info().
				Interface("response", response).
				Str("repository", repository).
				Err(err).
				Int64("pull_request_id", pullRequest.ID).
				Msg("Failed to merge pull-request")
			return releaseText, err
		}

		log.Logger().Info().
			Interface("response", response).
			Int64("pull_request_id", pullRequest.ID).
			Msg("Merged pull-request")
	}

	if len(pullRequests) == 1 {
		releaseText += fmt.Sprintf("\nI merged pull-request #`%d` into destination branch of repository `%s` :)", lastPullRequest.ID, repository)
		return releaseText, nil
	}

	releaseText += fmt.Sprintf("\nI merged all pull-requests for repository `%s` into destination branch :)", repository)

	return releaseText, nil
}

func checkIfRequiredReviewersExists(info dto.BitBucketPullRequestInfoResponse) (bool, failedToMerge) {
	requiredReviewers := container.C.Config.BitBucketConfig.RequiredReviewers

	var (
		existsInReviewers = false
		result            = failedToMerge{}
	)

	for _, reviewerUuID := range requiredReviewers {
		if existsInReviewers == true {
			break
		}

		for _, user := range info.Participants {
			if reviewerUuID.UUID == user.User.UUID {
				existsInReviewers = true
			}
		}
	}

	if existsInReviewers == false {
		var reviewersSlackUsers = make([]string, len(requiredReviewers))
		for i, reviewerUuID := range requiredReviewers {
			reviewersSlackUsers[i] = fmt.Sprintf("<@%s>", reviewerUuID.SlackUID)
		}

		result = failedToMerge{
			Reason: fmt.Sprintf("One of the required reviewers (%s) was not found in the reviewers list.", strings.Join(reviewersSlackUsers, ", ")),
			Info:   info,
			Error:  nil,
		}
		return false, result
	}

	return true, failedToMerge{}
}

func checkIfOneOfRequiredReviewersApprovedPullRequest(info dto.BitBucketPullRequestInfoResponse) bool {
	requiredReviewers := container.C.Config.BitBucketConfig.RequiredReviewers

	var isPullRequestApprovedByReviewers = false
	for _, reviewerUuID := range requiredReviewers {

		for _, user := range info.Participants {
			if reviewerUuID.UUID == user.User.UUID && user.Approved {
				isPullRequestApprovedByReviewers = true
			}
		}
	}

	return isPullRequestApprovedByReviewers
}

func isPullRequestAlreadyMerged(info dto.BitBucketPullRequestInfoResponse) bool {
	if info.State == pullRequestStateOpen {
		return true
	}

	return false
}

func receivedPullRequestsText(foundPullRequests ReceivedPullRequests) string {

	if len(foundPullRequests.Items) == 0 {
		return noPullRequestStringAnswer
	}

	var pullRequestsString = pullRequestStringAnswer
	for _, item := range foundPullRequests.Items {
		pullRequestsString = pullRequestsString + fmt.Sprintf("Pull-request #%d [repository: %s]\n", item.ID, item.RepositorySlug)
	}

	return pullRequestsString
}

func findAllPullRequestsInText(regex string, subject string) ReceivedPullRequests {
	re, err := regexp.Compile(regex)

	if err != nil {
		log.Logger().AddError(err).Msg("Error during the Find Matches operation")
		return ReceivedPullRequests{}
	}

	matches := re.FindAllStringSubmatch(subject, -1)
	result := ReceivedPullRequests{}

	if len(matches) == 0 {
		return result
	}

	for _, id := range matches {
		if id[1] != "" {
			item := PullRequest{}
			item.Workspace = id[1]
			item.RepositorySlug = id[2]
			item.ID, err = strconv.ParseInt(id[3], 10, 64)
			if err != nil {
				log.Logger().AddError(err).
					Interface("matches", matches).
					Msg("Error during pull-request ID parsing")
				return ReceivedPullRequests{}
			}

			result.Items = append(result.Items, item)
		}
	}

	return result
}

func prepareReleaseTitle(currentTitle string) string {
	if !strings.Contains(currentTitle, "[PREPARED-FOR-RELEASE]") {
		return fmt.Sprintf("[PREPARED-FOR-RELEASE] %s", currentTitle)
	}

	return currentTitle
}
